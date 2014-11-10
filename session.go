package imapsrv

import (
	"fmt"
	"log"
	"regexp"
)

// IMAP session states
type state int

const (
	notAuthenticated = iota
	authenticated
	selected
)

// An IMAP session
type session struct {
	// The client id
	id int
	// The state of the session
	st state
	// The currently selected mailbox (if st == selected)
	mailbox *Mailbox
	// IMAP configuration
	config *Config
}

// Create a new IMAP session
func createSession(id int, config *Config) *session {
	return &session{
		id:     id,
		st:     notAuthenticated,
		config: config}
}

// Log a message with session information
func (s *session) log(info ...interface{}) {
	preamble := fmt.Sprintf("IMAP (%d) ", s.id)
	message := []interface{}{preamble}
	message = append(message, info...)
	log.Print(message...)
}

// Select a mailbox - returns true if the mailbox exists
func (s *session) selectMailbox(name string) (bool, error) {
	// Lookup the mailbox
	mailstore := s.config.Mailstore
	mbox, err := mailstore.GetMailbox(name)

	if err != nil {
		return false, err
	}

	if mbox == nil {
		return false, nil
	}

	// Make note of the mailbox
	s.mailbox = mbox
	return true, nil
}

// List mailboxes matching the given mailbox pattern
func (s *session) list(reference []string, mbox []string) ([]*Mailbox, error) {

	if len(mbox) == 0  {
		// Lookup mailboxes at the reference point
		return s.listMailboxes(reference, nil, false)
	} 

	// Will this be a recursive listing?
	recursive := false
	mboxLastIndex := len(mbox) - 1
	mboxPattern := mbox[mboxLastIndex]

	patLastIndex := len(mboxPattern) - 1
	if mboxPattern[patLastIndex] == '*' {
		recursive = true
	}

	// We will build a regular expression to match mailbox names
	// and a path to search from
	var mboxRe *regexp.Regexp = nil
	path := reference

	// Does the mailbox end in a wildcard?
	if mboxPattern[patLastIndex] == '*' || mboxPattern[patLastIndex] == '%' {

		// Build the mailbox path
		if mboxLastIndex > 0 {
			path = append(path, mbox[0:mboxLastIndex]...)
		}

		// Convert the wildcard into a regular expression
		var expr string
		if len(mboxPattern) == 1 {
			expr = ".*"
		} else {
			expr = mboxPattern[0:(patLastIndex-1)] + ".*"
		}

		var err error
		mboxRe, err = regexp.Compile(expr)

		if err != nil {
			return nil, err
		}

	} else {
		// Build the mailbox path
		path = append(path, mboxPattern)
	}

	// Lookup mailboxes at the given path
	return s.listMailboxes(path, mboxRe, recursive)

}

// Add mailbox information to the given response
func (s *session) addMailboxInfo(resp *response) error {
	mailstore := s.config.Mailstore

	// Get the mailbox information from the mailstore
	firstUnseen, err := mailstore.FirstUnseen(s.mailbox.Id)
	if err != nil {
		return err
	}
	totalMessages, err := mailstore.TotalMessages(s.mailbox.Id)
	if err != nil {
		return err
	}
	recentMessages, err := mailstore.RecentMessages(s.mailbox.Id)
	if err != nil {
		return err
	}
	nextUid, err := mailstore.NextUid(s.mailbox.Id)
	if err != nil {
		return err
	}

	resp.extra(fmt.Sprint(totalMessages, " EXISTS"))
	resp.extra(fmt.Sprint(recentMessages, " RECENT"))
	resp.extra(fmt.Sprintf("OK [UNSEEN %d] Message %d is first unseen", firstUnseen, firstUnseen))
	resp.extra(fmt.Sprintf("OK [UIDVALIDITY %d] UIDs valid", s.mailbox.Id))
	resp.extra(fmt.Sprintf("OK [UIDNEXT %d] Predicted next UID", nextUid))
	return nil
}

// Recursive list mailboxes function.
func (s *session) listMailboxes(
	path []string,
	mboxRe *regexp.Regexp,
	recursive bool) ([]*Mailbox, error) {

	log.Print("listMailboxes ", path)

	// Lookup mailboxes at the given path
	mailstore := s.config.Mailstore
	current, err := mailstore.GetMailboxes(path)

	if err != nil {
		return nil, err
	}

	// Loop through the results
	ret := make([]*Mailbox, 0, 4)

	for _, mbox := range current {

		// Is there a pattern to match?
		if mboxRe != nil && !mboxRe.MatchString(mbox.Name) {
			continue
		}

		// Add the mailbox to the results
		ret = append(ret, mbox)

		// Is this a recursive listing?
		if recursive {
			// Build the path to the child mailbox
			nextPath := append(path, mbox.Name)

			// List the mailboxes in the child mailbox
			next, err := s.listMailboxes(nextPath, nil, true)
			if err != nil {
				return nil, err
			}
			ret = append(ret, next...)
		}
	}

	return ret, nil
}
