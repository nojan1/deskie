package main

import (
	"errors"
	"net/url"
	"strings"
)

// redact removes a secret URL (and its path, which is where Google puts the
// private token) from an error message before it reaches the journal.
func redact(err error, secret string) error {
	msg := err.Error()
	if secret != "" {
		msg = strings.ReplaceAll(msg, secret, "<secret url>")
		if u, perr := url.Parse(secret); perr == nil && u.Path != "" && u.Path != "/" {
			msg = strings.ReplaceAll(msg, u.Path, "/<secret>")
		}
	}
	return errors.New(msg)
}
