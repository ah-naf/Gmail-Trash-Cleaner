package main

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset" // decode non-UTF-8 bodies
	"github.com/emersion/go-message/mail"
)

var (
	reBlocks   = regexp.MustCompile(`(?is)<(script|style|head)[^>]*>.*?</(script|style|head)>`)
	reBreaks   = regexp.MustCompile(`(?i)<(br|/p|/div|/tr|/li|/h[1-6])[^>]*>`)
	reTags     = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlankRun = regexp.MustCompile(`\n[ \t]*(\n[ \t]*)+`)
	reSpaces   = regexp.MustCompile(`[ \t\x{00a0}]+`)
)

// subjectAndDate returns the subject and a YYYY-MM-DD date for naming files.
func subjectAndDate(raw []byte) (subject, date string) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if mr == nil || (err != nil && !message.IsUnknownCharset(err)) {
		return "", ""
	}
	defer mr.Close()
	subject, _ = mr.Header.Subject()
	if t, err := mr.Header.Date(); err == nil && !t.IsZero() {
		date = t.Format("2006-01-02")
	}
	return subject, date
}

// attachment is a file attached to a message.
type attachment struct {
	Name string
	Data []byte
}

// renderReadable turns a raw RFC 822 message into plain text: key headers,
// the text body (HTML is converted if there is no plain part) and a list of
// attachment names. The attachments themselves are returned separately.
func renderReadable(raw []byte) (string, []attachment, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if mr == nil || (err != nil && !message.IsUnknownCharset(err)) {
		return "", nil, err
	}
	defer mr.Close()

	var sb strings.Builder
	h := mr.Header
	subject, _ := h.Subject()
	fmt.Fprintf(&sb, "Subject: %s\n", subject)
	for _, field := range []string{"From", "To", "Cc"} {
		if addrs, err := h.AddressList(field); err == nil && len(addrs) > 0 {
			parts := make([]string, len(addrs))
			for i, a := range addrs {
				parts[i] = a.String()
			}
			fmt.Fprintf(&sb, "%s: %s\n", field, strings.Join(parts, ", "))
		}
	}
	if t, err := h.Date(); err == nil && !t.IsZero() {
		fmt.Fprintf(&sb, "Date: %s\n", t.Format("2006-01-02 15:04 MST"))
	}

	var plain, htmlBody string
	var attachments []attachment
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil && !message.IsUnknownCharset(err) {
			break // keep whatever was parsed so far
		}
		switch ph := p.Header.(type) {
		case *mail.InlineHeader:
			ct, params, _ := ph.ContentType()
			body, _ := io.ReadAll(p.Body)
			switch ct {
			case "text/plain":
				plain += string(body)
			case "text/html":
				htmlBody += string(body)
			default:
				// Inline images and other non-text parts (e.g. a picture
				// pasted into the message) are not marked as attachments.
				if len(body) == 0 {
					break
				}
				name := params["name"]
				if _, dp, err := ph.ContentDisposition(); err == nil && dp["filename"] != "" {
					name = dp["filename"]
				}
				if name == "" {
					name = "inline" + inlineExt(ct)
				}
				attachments = append(attachments, attachment{Name: name, Data: body})
			}
		case *mail.AttachmentHeader:
			name, _ := ph.Filename()
			if name == "" {
				name = "(unnamed)"
			}
			data, _ := io.ReadAll(p.Body)
			attachments = append(attachments, attachment{Name: name, Data: data})
		}
	}

	text := strings.TrimSpace(plain)
	if text == "" {
		text = htmlToText(htmlBody)
	}
	sb.WriteString("\n" + text + "\n")
	if len(attachments) > 0 {
		sb.WriteString("\nAttachments:\n")
		for _, a := range attachments {
			sb.WriteString("  - " + a.Name + "\n")
		}
	}
	return sb.String(), attachments, nil
}

func htmlToText(s string) string {
	s = reBlocks.ReplaceAllString(s, "")
	s = reBreaks.ReplaceAllString(s, "\n")
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r", "")
	s = reSpaces.ReplaceAllString(s, " ")
	s = reBlankRun.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var reUnsafe = regexp.MustCompile(`[^\p{L}\p{N} ._-]+`)

// fileName builds "<date> <subject> [<uid>]" with filesystem-safe characters.
func fileName(date, subject string, uid uint32) string {
	subject = strings.TrimSpace(reUnsafe.ReplaceAllString(subject, ""))
	if subject == "" {
		subject = "no subject"
	}
	if r := []rune(subject); len(r) > 60 {
		subject = strings.TrimSpace(string(r[:60]))
	}
	if date == "" {
		date = "undated"
	}
	return fmt.Sprintf("%s %s [%d]", date, subject, uid)
}

// saveAttachments writes the attachments into dir, prefixing names with an
// index so duplicate or unsafe names cannot collide or escape the directory.
func saveAttachments(dir string, atts []attachment) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i, a := range atts {
		name := reUnsafe.ReplaceAllString(filepath.Base(a.Name), "_")
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d-%s", i+1, name)), a.Data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// inlineExt returns a file extension for a MIME type, or "" if unknown.
func inlineExt(contentType string) string {
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		return exts[len(exts)-1]
	}
	return ""
}
