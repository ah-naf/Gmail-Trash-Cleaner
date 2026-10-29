# gmail-trash-cleaner

Backs up and then permanently deletes everything in a Gmail account's Trash, over IMAP. No browser login: it uses a Google app password. It can run by hand or as a macOS background job.

> **Deletion is permanent.** Mail removed from Trash cannot be recovered. Use `-dry-run` or `-export` first.

## Requirements

- Go 1.22+ (to build)
- A Gmail account with 2-Step Verification on
- macOS (only for the background job; the program itself runs anywhere)

## 1. Gmail setup (once)

1. Create an app password at https://myaccount.google.com/apppasswords (copy the 16 characters, no spaces).
2. In Gmail, go to **Settings → See all settings → Forwarding and POP/IMAP** and make sure IMAP is enabled.
3. Under **Settings → Labels**, make sure **Bin/Trash** has **Show in IMAP** ticked.

## 2. Build

```
go build -o gmail-trash-cleaner .
```

## 3. Run by hand

```
export GMAIL_USER='you@gmail.com'
export GMAIL_APP_PASSWORD='abcdefghijklmnop'

./gmail-trash-cleaner -dry-run                       # list date | sender | subject
./gmail-trash-cleaner -export ./backup               # save readable .txt files, no deletion
./gmail-trash-cleaner -export ./backup -attachments  # also save attachments and inline images
./gmail-trash-cleaner -export ./backup -raw          # also keep the original .eml files
./gmail-trash-cleaner                                # ask for "yes", then delete everything
./gmail-trash-cleaner -yes                           # delete without asking
```

| Flag | Meaning |
|---|---|
| `-dry-run` | List messages, change nothing |
| `-export DIR` | Save each message as `<date> <subject> [<uid>].txt`; never deletes |
| `-attachments` | With `-export`, save attachments into `<name> attachments/` |
| `-raw` | With `-export`, also save the original `.eml` |
| `-yes` | Skip the confirmation prompt |

Set `IMAP_DEBUG=1` to print the IMAP conversation (it includes your password in the `LOGIN` line, so do not share it).

## 4. Run in the background (macOS)

The job exports Trash into a folder named after the time window it covers, then empties Trash. It runs every 60 seconds, at login, and after every restart.

1. **Edit the two files in `launchd/`** to match your account:
   - `run-cleaner.sh`: change `GMAIL_USER` to your address.
   - `com.testreco.gmail-trash-cleaner.plist`: replace `/Users/testreco` with your home folder (3 places). Change `StartInterval` (seconds) if you want a different interval.
2. **Store the app password in the Keychain** (prompts for it; use your address):
   ```
   security add-generic-password -a "you@gmail.com" -s gmail-trash-cleaner -w
   ```
3. **Install** (outside `~/Downloads`, which background jobs cannot read):
   ```
   mkdir -p ~/bin ~/Library/LaunchAgents
   cp gmail-trash-cleaner launchd/run-cleaner.sh ~/bin/
   cp launchd/com.testreco.gmail-trash-cleaner.plist ~/Library/LaunchAgents/
   ```
4. **Test once by hand** (click **Always Allow** if Keychain asks):
   ```
   ~/bin/run-cleaner.sh
   ```
5. **Start the job:**
   ```
   launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.testreco.gmail-trash-cleaner.plist
   ```

### Where things go

| What | Where |
|---|---|
| Log | `~/Library/Logs/gmail-trash-cleaner.log` |
| Exports | `~/gmail-trash-backup/<from>_to_<to>/` (timestamps `YYYY-MM-DD_HHMMSS`) |
| Last successful run | `~/gmail-trash-backup/.last-run` |

A minute with nothing in Trash creates no folder. If a run fails, the window start does not move, so the next folder covers the gap.

### Useful commands

```
launchctl print gui/$(id -u)/com.testreco.gmail-trash-cleaner | grep -E "state|runs|last exit"
launchctl kickstart -k gui/$(id -u)/com.testreco.gmail-trash-cleaner   # run now
tail -f ~/Library/Logs/gmail-trash-cleaner.log
```

After changing the script, binary or plist: copy the new file to its install location, then `launchctl bootout gui/$(id -u)/com.testreco.gmail-trash-cleaner` and run the bootstrap command again.

### Stop and remove

```
launchctl bootout gui/$(id -u)/com.testreco.gmail-trash-cleaner
rm ~/Library/LaunchAgents/com.testreco.gmail-trash-cleaner.plist
```

## Troubleshooting

- **`find trash: no mailbox with \Trash attribute`**: Trash is hidden from IMAP; tick **Show in IMAP** (step 1.3).
- **`login: ... Invalid credentials`**: wrong app password, or 2-Step Verification is off. Enter the password without spaces.
- **Keychain lookup fails**: the item name must be `gmail-trash-cleaner` and the account must match `GMAIL_USER` exactly. Check with `security find-generic-password -a "you@gmail.com" -s gmail-trash-cleaner -w`.
- **Job shows `runs = 0`**: make sure the plist has `RunAtLoad`, then reload it.
- **Phone still shows deleted mail**: the Gmail app caches; pull to refresh or re-sync the account.

## Notes

- The job only runs while you are logged in (the Keychain is locked before login). Enable automatic login if you need it to start after an unattended reboot.
- It logs in to Gmail every minute. If you see login errors, increase `StartInterval`.
- Mail that lands in Trash between the export and the delete step of one run is deleted without being exported (a window of a few seconds).
- Do not commit `GMAIL_APP_PASSWORD` or exported mail to version control.
