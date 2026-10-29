// Command gmail-trash-cleaner permanently deletes every message in a Gmail
// account's Trash over IMAP. It needs no browser: authenticate with a Google
// app password supplied through environment variables.
//
//	GMAIL_USER          full Gmail address
//	GMAIL_APP_PASSWORD  16-character app password
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "list trash messages (date, sender, subject) without deleting them")
	exportDir := flag.String("export", "", "save every trash message as a readable .txt file in this directory (does not delete)")
	rawEml := flag.Bool("raw", false, "with -export, also save the original .eml (keeps attachments)")
	attachments := flag.Bool("attachments", false, "with -export, also save attachments into a folder next to each .txt")
	yes := flag.Bool("yes", false, "skip the confirmation prompt")
	flag.Parse()

	user, pass := os.Getenv("GMAIL_USER"), os.Getenv("GMAIL_APP_PASSWORD")
	if user == "" || pass == "" {
		log.Fatal("set GMAIL_USER and GMAIL_APP_PASSWORD")
	}

	opts := &imapclient.Options{}
	if os.Getenv("IMAP_DEBUG") != "" {
		opts.DebugWriter = os.Stderr
	}
	c, err := imapclient.DialTLS("imap.gmail.com:993", opts)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if err := c.Login(user, pass).Wait(); err != nil {
		log.Fatalf("login: %v", err)
	}
	defer func() { c.Logout().Wait() }()

	trash, err := trashMailbox(c)
	if err != nil {
		log.Fatalf("find trash: %v", err)
	}

	sel, err := c.Select(trash, nil).Wait()
	if err != nil {
		log.Fatalf("select %q: %v", trash, err)
	}
	fmt.Printf("Found %d message(s) in %s.\n", sel.NumMessages, trash)
	if sel.NumMessages == 0 {
		return
	}

	all := imap.SeqSet{}
	all.AddRange(1, 0) // 1:* (every message)

	if *exportDir != "" {
		n, err := exportMessages(c, all, *exportDir, *rawEml, *attachments)
		if err != nil {
			log.Fatalf("export (after %d saved): %v", n, err)
		}
		fmt.Printf("Exported %d message(s) to %s. Nothing was deleted.\n", n, *exportDir)
		return
	}

	if *dryRun {
		if err := printMessages(c, all); err != nil {
			log.Fatalf("list messages: %v", err)
		}
		fmt.Println("Dry run: nothing was deleted.")
		return
	}

	if !*yes {
		fmt.Print("Permanently delete them? This cannot be undone. Type 'yes' to continue: ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	store := &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}
	if err := c.Store(all, store, nil).Close(); err != nil {
		log.Fatalf("flag messages: %v", err)
	}
	// Expunging inside Gmail's Trash removes the messages permanently.
	if _, err := c.Expunge().Collect(); err != nil {
		log.Fatalf("expunge: %v", err)
	}
	fmt.Printf("Deleted %d message(s). Trash emptied.\n", sel.NumMessages)
}

// trashMailbox finds the mailbox with the \Trash special-use attribute, so it
// works regardless of the account language.
func trashMailbox(c *imapclient.Client) (string, error) {
	// Only list Gmail's system folders; user labels are irrelevant here.
	boxes, err := c.List("", "[Gmail]/*", nil).Collect()
	if err != nil {
		return "", err
	}
	for _, b := range boxes {
		if slices.Contains(b.Attrs, imap.MailboxAttrTrash) {
			return b.Mailbox, nil
		}
	}
	for _, b := range boxes {
		fmt.Fprintf(os.Stderr, "mailbox %q attrs=%v\n", b.Mailbox, b.Attrs)
	}
	return "", fmt.Errorf("no mailbox with \\Trash attribute (%d listed)", len(boxes))
}

// exportMessages saves each message as a readable "<date> <subject> [<uid>].txt"
// in dir (plus the original .eml when raw is set or parsing fails) and returns
// how many messages were written.
func exportMessages(c *imapclient.Client, set imap.SeqSet, dir string, raw, saveAtts bool) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	fetch := c.Fetch(set, &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{Peek: true}}, // whole message, keep unread state
	})
	defer fetch.Close()

	n := 0
	for {
		msg := fetch.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			return n, err
		}
		if len(buf.BodySection) == 0 {
			return n, fmt.Errorf("message %d: server returned no body", buf.SeqNum)
		}
		src := buf.BodySection[0].Bytes
		keepRaw := raw
		subject, date := subjectAndDate(src)
		base := filepath.Join(dir, fileName(date, subject, uint32(buf.UID)))
		text, atts, err := renderReadable(src)
		if err != nil {
			text = fmt.Sprintf("(could not parse message: %v; see the .eml file)\n", err)
			keepRaw = true
		}
		if err := os.WriteFile(base+".txt", []byte(text), 0o600); err != nil {
			return n, err
		}
		if saveAtts && len(atts) > 0 {
			if err := saveAttachments(base+" attachments", atts); err != nil {
				return n, err
			}
		}
		if keepRaw {
			if err := os.WriteFile(base+".eml", src, 0o600); err != nil {
				return n, err
			}
		}
		n++
		if n%50 == 0 {
			fmt.Printf("Exported %d...\n", n)
		}
	}
	return n, fetch.Close()
}

// printMessages prints date, sender and subject for the given messages.
func printMessages(c *imapclient.Client, set imap.SeqSet) error {
	fetch := c.Fetch(set, &imap.FetchOptions{Envelope: true})
	defer fetch.Close()

	n := 0
	for {
		msg := fetch.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			return err
		}
		n++
		env := buf.Envelope
		from := ""
		if env != nil && len(env.From) > 0 {
			from = env.From[0].Addr()
		}
		var date, subject string
		if env != nil {
			date, subject = env.Date.Format("2006-01-02 15:04"), env.Subject
		}
		fmt.Printf("%4d. %s | %s | %s\n", n, date, from, subject)
	}
	return fetch.Close()
}
