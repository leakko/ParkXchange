package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

// Credential limits.
//
// The password floor is a length requirement and nothing else. Composition
// rules ("one capital, one symbol") reliably produce "Password1!" and measurably
// reduce entropy by funnelling everyone into the same shapes, so they are not
// imposed here.
const (
	MinPasswordLength = 10

	// MaxPasswordLength exists because argon2id is deliberately expensive. An
	// unbounded password is a free way to make the server burn CPU and memory
	// on demand.
	MaxPasswordLength = 256

	// MaxEmailLength is the limit RFC 5321 puts on a forward path.
	MaxEmailLength = 254

	MinDisplayNameLength = 2
	MaxDisplayNameLength = 60
)

// Email is a normalised, syntactically valid address.
//
// It is a distinct type so that an unvalidated string cannot be passed where
// an address is expected. The compiler then enforces what would otherwise be a
// convention nobody remembers at the fourth call site.
type Email string

// ParseEmail normalises and validates an address.
//
// Normalisation lowercases, because the unique index in the database is on
// lower(email). If the two disagreed, somebody could register the same address
// twice in different cases, or register successfully and then fail to log in.
func ParseEmail(raw string) (Email, error) {
	normalised := strings.ToLower(strings.TrimSpace(raw))

	switch {
	case normalised == "":
		return "", Invalid("email_required", "an email address is required")
	case len(normalised) > MaxEmailLength:
		return "", Invalid("email_too_long", "that email address is too long")
	}

	if _, err := mail.ParseAddress(normalised); err != nil {
		return "", Invalid("email_invalid", "that is not a valid email address")
	}

	return Email(normalised), nil
}

// NewEmail trusts an address that has already been persisted.
//
// Values read back from the database were validated on the way in, and
// re-validating them would mean a row that a stricter future rule rejects
// becomes unreadable rather than merely unwritable.
func NewEmail(stored string) Email { return Email(stored) }

func (e Email) String() string { return string(e) }

// User is an account.
//
// PasswordHash is part of the entity because verifying a password is a domain
// operation, but note that nothing in this package can serialise it: rendering
// a user for the wire happens in the HTTP adapter, which builds its own
// response shape and simply never reads this field.
type User struct {
	ID           string
	Email        Email
	PasswordHash string
	DisplayName  string
	RatingSum    int
	RatingCount  int
	BalanceCents int64
	CreatedAt    time.Time
}

// Rating returns the average rating and whether the user has been rated.
//
// The boolean matters: a user with no ratings is not a zero-star user, and
// collapsing the two would punish every new account. Callers render the
// difference.
func (u User) Rating() (average float64, rated bool) {
	if u.RatingCount == 0 {
		return 0, false
	}
	return float64(u.RatingSum) / float64(u.RatingCount), true
}

// Claims identifies whoever is making the current request.
//
// The name is borrowed from JWT, but the concept is not: this is simply "who
// is acting", and it would look the same if sessions were cookies in a table.
// It lives in the domain because authorisation rules are domain rules, and a
// rule like "only the owner may delete a spot" needs to name the actor.
type Claims struct {
	UserID string
	Email  string
}

// Authenticated reports whether the claims identify anybody.
//
// A use case must check this rather than assume, so that a handler reached
// without the auth middleware fails loudly instead of quietly acting as the
// zero-value user, which would be an authorisation bypass.
func (c Claims) Authenticated() bool { return c.UserID != "" }

// NewUserInput is a registration request that has not been checked yet.
type NewUserInput struct {
	Email       string
	Password    string
	DisplayName string
}

// NewUser validates a registration and returns the accepted values.
//
// Every field is checked before returning, rather than failing on the first
// problem, so a client can show all of them at once instead of making the user
// resubmit three times to discover three mistakes.
func NewUser(in NewUserInput) (email Email, displayName string, err error) {
	fields := make(map[string]string)

	parsedEmail, emailErr := ParseEmail(in.Email)
	if emailErr != nil {
		if domainErr, ok := AsError(emailErr); ok {
			fields["email"] = domainErr.Message
		} else {
			fields["email"] = "is not valid"
		}
	}

	if problem := validatePassword(in.Password); problem != "" {
		fields["password"] = problem
	}

	name := strings.TrimSpace(in.DisplayName)
	if problem := validateDisplayName(name); problem != "" {
		fields["display_name"] = problem
	}

	if len(fields) > 0 {
		return "", "", InvalidFields(fields)
	}

	return parsedEmail, name, nil
}

// ParseDisplayName trims and validates a display name for create or update.
func ParseDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if problem := validateDisplayName(name); problem != "" {
		return "", InvalidFields(map[string]string{"display_name": problem})
	}
	return name, nil
}

// PasswordProblem reports why a password fails the length rules, or "" if it
// is acceptable. Callers choose the field name in InvalidFields so register
// and password-change can disagree about the wire key.
func PasswordProblem(password string) string {
	return validatePassword(password)
}

func validatePassword(password string) string {
	switch {
	case password == "":
		return "is required"

	// Counted in runes, not bytes, so a passphrase in a non-Latin script is
	// not rejected for being "short" when it is nothing of the sort.
	case utf8.RuneCountInString(password) < MinPasswordLength:
		return "must be at least 10 characters"

	// Bounded in bytes, because the cost being defended against is the work
	// argon2 does over the raw input.
	case len(password) > MaxPasswordLength:
		return "is too long"
	}
	return ""
}

func validateDisplayName(name string) string {
	length := utf8.RuneCountInString(name)
	switch {
	case name == "":
		return "is required"
	case length < MinDisplayNameLength:
		return "must be at least 2 characters"
	case length > MaxDisplayNameLength:
		return "must be at most 60 characters"
	}
	return ""
}
