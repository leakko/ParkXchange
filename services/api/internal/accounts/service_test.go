package accounts_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/marco/parkxchange/services/api/internal/accounts"
	"github.com/marco/parkxchange/services/api/internal/domain"
)

// fakeStore records what the service asked for and returns what the test
// arranges. It checks use-case decisions, not SQL.
type fakeStore struct {
	users map[string]domain.User

	updatedDisplayName  string
	updatedLocale       string
	updatedPasswordHash string
	revokedAllFor       string
	closedAccountFor    string

	refreshTokens map[string][][]byte // userID → hashes
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:         make(map[string]domain.User),
		refreshTokens: make(map[string][][]byte),
	}
}

func (f *fakeStore) CreateUser(_ context.Context, email domain.Email, passwordHash, displayName string, phone domain.Phone) (domain.User, error) {
	user := domain.User{
		ID:           "user-1",
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
		Phone:        phone,
	}
	f.users[user.ID] = user
	return user, nil
}

func (f *fakeStore) UserByEmail(_ context.Context, email domain.Email) (domain.User, error) {
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNoRows
}

func (f *fakeStore) UserByID(_ context.Context, id string) (domain.User, error) {
	u, found := f.users[id]
	if !found {
		return domain.User{}, domain.ErrNoRows
	}
	return u, nil
}

func (f *fakeStore) InsertRefreshToken(_ context.Context, userID string, tokenHash []byte, _ time.Time, _ string) error {
	f.refreshTokens[userID] = append(f.refreshTokens[userID], tokenHash)
	return nil
}

func (f *fakeStore) RotateRefreshToken(context.Context, []byte, []byte, time.Time, string) (domain.User, error) {
	return domain.User{}, domain.ErrNoRows
}

func (f *fakeStore) RevokeRefreshToken(context.Context, []byte) error { return nil }

func (f *fakeStore) UpdateDisplayName(_ context.Context, userID, displayName string) (domain.User, error) {
	u, found := f.users[userID]
	if !found {
		return domain.User{}, domain.ErrNoRows
	}
	u.DisplayName = displayName
	f.users[userID] = u
	f.updatedDisplayName = displayName
	return u, nil
}

func (f *fakeStore) ChangePassword(_ context.Context, userID, passwordHash string) error {
	u, found := f.users[userID]
	if !found {
		return domain.ErrNoRows
	}
	u.PasswordHash = passwordHash
	f.users[userID] = u
	f.updatedPasswordHash = passwordHash
	f.revokedAllFor = userID
	delete(f.refreshTokens, userID)
	return nil
}

func (f *fakeStore) UserByGoogleSub(_ context.Context, googleSub string) (domain.User, error) {
	for _, u := range f.users {
		if u.GoogleSub == googleSub {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrNoRows
}

func (f *fakeStore) LinkGoogleSub(_ context.Context, userID, googleSub string) (domain.User, error) {
	u, found := f.users[userID]
	if !found {
		return domain.User{}, domain.ErrNoRows
	}
	for _, other := range f.users {
		if other.ID != userID && other.GoogleSub == googleSub {
			return domain.User{}, domain.ErrDuplicate
		}
	}
	u.GoogleSub = googleSub
	f.users[userID] = u
	return u, nil
}

func (f *fakeStore) UpdatePhone(_ context.Context, userID string, phone domain.Phone) (domain.User, error) {
	u, found := f.users[userID]
	if !found {
		return domain.User{}, domain.ErrNoRows
	}
	u.Phone = phone
	f.users[userID] = u
	return u, nil
}

func (f *fakeStore) UpdateLocale(_ context.Context, userID string, locale domain.Locale) (domain.User, error) {
	u, found := f.users[userID]
	if !found {
		return domain.User{}, domain.ErrNoRows
	}
	u.Locale = locale
	f.users[userID] = u
	f.updatedLocale = locale.String()
	return u, nil
}

func (f *fakeStore) InsertPasswordResetToken(context.Context, string, []byte, time.Time) error {
	return nil
}

func (f *fakeStore) CompletePasswordReset(context.Context, []byte, string) error {
	return domain.ErrNoRows
}

func (f *fakeStore) MarkEmailVerified(_ context.Context, userID string) (domain.User, error) {
	u, found := f.users[userID]
	if !found {
		return domain.User{}, domain.ErrNoRows
	}
	now := time.Now()
	u.EmailVerifiedAt = &now
	f.users[userID] = u
	return u, nil
}

func (f *fakeStore) InvalidateOpenEmailVerificationTokens(context.Context, string) error {
	return nil
}

func (f *fakeStore) InsertEmailVerificationToken(context.Context, string, []byte, time.Time) error {
	return nil
}

func (f *fakeStore) LatestEmailVerificationCreatedAt(context.Context, string) (*time.Time, error) {
	return nil, domain.ErrNoRows
}

func (f *fakeStore) CompleteEmailVerification(context.Context, []byte) (domain.User, error) {
	return domain.User{}, domain.ErrNoRows
}

func (f *fakeStore) CloseAccount(_ context.Context, userID string) error {
	if _, found := f.users[userID]; !found {
		return domain.ErrNoRows
	}
	f.closedAccountFor = userID
	delete(f.users, userID)
	delete(f.refreshTokens, userID)
	return nil
}

func (f *fakeStore) UpsertPushToken(context.Context, string, string, string) error {
	return nil
}
func (f *fakeStore) ListRatingsForUser(context.Context, string, int, int) ([]domain.Rating, error) {
	return nil, nil
}
func (f *fakeStore) InsertReport(context.Context, domain.ReportDraft) error {
	return nil
}

// plainHasher stores the password itself so unit tests stay cheap.
type plainHasher struct{}

func (plainHasher) Hash(password string) (string, error) { return "hash:" + password, nil }

func (plainHasher) Verify(password, hash string) (bool, error) {
	return hash == "hash:"+password, nil
}

type stubTokens struct{}

func (stubTokens) IssueAccess(userID, email string) (string, time.Time, error) {
	return "access-" + userID, time.Now().Add(time.Hour), nil
}
func (stubTokens) ParseAccess(string) (domain.Claims, error) { return domain.Claims{}, nil }
func (stubTokens) NewRefreshToken() (string, []byte, error) {
	return "refresh", []byte("refresh-hash"), nil
}
func (stubTokens) HashRefreshToken(plaintext string) []byte { return []byte("hash:" + plaintext) }
func (stubTokens) IssueSocketTicket(userID, email string) (string, time.Time, error) {
	return "ticket", time.Now().Add(time.Minute), nil
}
func (stubTokens) ParseSocketTicket(string) (domain.Claims, error) {
	return domain.Claims{}, nil
}

func newService(t *testing.T, store accounts.Store) *accounts.Service {
	t.Helper()
	svc, err := accounts.New(store, plainHasher{}, stubTokens{}, 24*time.Hour, nil, nil, "", "")
	if err != nil {
		t.Fatalf("accounts.New: %v", err)
	}
	return svc
}

func seededUser(store *fakeStore) domain.User {
	user := domain.User{
		ID:           "user-1",
		Email:        domain.NewEmail("marco@parkxchange.invalid"),
		PasswordHash: "hash:current-password-ok",
		DisplayName:  "Old Name",
	}
	store.users[user.ID] = user
	return user
}

func TestUpdateDisplayNamePersistsTheTrimmedName(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)
	viewer := domain.Claims{UserID: "user-1", Email: "marco@parkxchange.invalid"}

	got, err := svc.UpdateDisplayName(context.Background(), viewer, "  New Name  ")
	if err != nil {
		t.Fatalf("UpdateDisplayName: %v", err)
	}
	if got.DisplayName != "New Name" {
		t.Errorf("DisplayName = %q, want New Name", got.DisplayName)
	}
	if store.updatedDisplayName != "New Name" {
		t.Errorf("store got %q, want trimmed New Name", store.updatedDisplayName)
	}
}

func TestUpdateDisplayNameRejectsAnEmptyName(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)

	_, err := svc.UpdateDisplayName(context.Background(),
		domain.Claims{UserID: "user-1"}, "   ")
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Fields["display_name"] == "" {
		t.Errorf("fields = %v, want display_name named", domainErr)
	}
	if store.updatedDisplayName != "" {
		t.Error("store was written despite invalid input")
	}
}

func TestUpdateDisplayNameRejectsANameThatIsTooLong(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)

	_, err := svc.UpdateDisplayName(context.Background(),
		domain.Claims{UserID: "user-1"}, strings.Repeat("x", 61))
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
}

func TestUpdateDisplayNameRequiresAuthentication(t *testing.T) {
	t.Parallel()

	svc := newService(t, newFakeStore())

	_, err := svc.UpdateDisplayName(context.Background(), domain.Claims{}, "Anyone")
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want KindUnauthenticated", domain.KindOf(err))
	}
}

func TestChangePasswordVerifiesHashesAndRevokesRefreshTokens(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	store.refreshTokens["user-1"] = [][]byte{[]byte("old-token")}
	svc := newService(t, store)

	err := svc.ChangePassword(context.Background(),
		domain.Claims{UserID: "user-1"},
		"current-password-ok",
		"a-brand-new-password")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if store.updatedPasswordHash != "hash:a-brand-new-password" {
		t.Errorf("password hash = %q, want hash of the new password", store.updatedPasswordHash)
	}
	if store.revokedAllFor != "user-1" {
		t.Errorf("ChangePassword revoked tokens for %q, want user-1", store.revokedAllFor)
	}
	if _, still := store.refreshTokens["user-1"]; still {
		t.Error("refresh tokens were not cleared")
	}
}

func TestChangePasswordRejectsAWrongCurrentPassword(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)

	err := svc.ChangePassword(context.Background(),
		domain.Claims{UserID: "user-1"},
		"definitely-wrong",
		"a-brand-new-password")
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want KindUnauthenticated like login", domain.KindOf(err))
	}
	if store.updatedPasswordHash != "" {
		t.Error("password was updated despite a wrong current password")
	}
	if store.revokedAllFor != "" {
		t.Error("tokens were revoked despite a wrong current password")
	}
}

func TestChangePasswordRejectsAShortNewPassword(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)

	err := svc.ChangePassword(context.Background(),
		domain.Claims{UserID: "user-1"},
		"current-password-ok",
		"short")
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Fields["new_password"] == "" {
		t.Errorf("fields = %v, want new_password named", domainErr)
	}
	if store.updatedPasswordHash != "" {
		t.Error("password was updated despite invalid new password")
	}
}

func TestChangePasswordRequiresAuthentication(t *testing.T) {
	t.Parallel()

	svc := newService(t, newFakeStore())

	err := svc.ChangePassword(context.Background(), domain.Claims{}, "x", "a-perfectly-fine-password")
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want KindUnauthenticated", domain.KindOf(err))
	}
}

func TestDeleteAccountCallsCloseAccount(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)

	err := svc.DeleteAccount(context.Background(), domain.Claims{UserID: "user-1"})
	if err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if store.closedAccountFor != "user-1" {
		t.Errorf("CloseAccount called for %q, want user-1", store.closedAccountFor)
	}
	if _, found := store.users["user-1"]; found {
		t.Error("user still present after CloseAccount")
	}
}

func TestDeleteAccountRequiresAuthentication(t *testing.T) {
	t.Parallel()

	svc := newService(t, newFakeStore())
	err := svc.DeleteAccount(context.Background(), domain.Claims{})
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want KindUnauthenticated", domain.KindOf(err))
	}
}

func TestDeleteAccountMissingUser(t *testing.T) {
	t.Parallel()

	svc := newService(t, newFakeStore())
	err := svc.DeleteAccount(context.Background(), domain.Claims{UserID: "missing"})
	if domain.KindOf(err) != domain.KindUnauthenticated {
		t.Errorf("kind = %v, want KindUnauthenticated", domain.KindOf(err))
	}
}

func TestCreateReportProblem(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	svc := newService(t, store)
	err := svc.CreateReport(
		context.Background(),
		domain.Claims{UserID: "user-1"},
		"something is wrong",
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateReportRejectsSelf(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	svc := newService(t, store)
	err := svc.CreateReport(
		context.Background(),
		domain.Claims{UserID: "user-1"},
		"bad",
		"",
		"user-1",
	)
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
}

func TestUpdateLocaleRejectsUnsupported(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)
	viewer := domain.Claims{UserID: "user-1", Email: "marco@parkxchange.invalid"}

	_, err := svc.UpdateLocale(context.Background(), viewer, "fr")
	if domain.KindOf(err) != domain.KindInvalid {
		t.Fatalf("kind = %v, want KindInvalid", domain.KindOf(err))
	}
	if store.updatedLocale != "" {
		t.Error("store was updated despite invalid locale")
	}
}

func TestUpdateLocalePersistsEnglish(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	seededUser(store)
	svc := newService(t, store)
	viewer := domain.Claims{UserID: "user-1", Email: "marco@parkxchange.invalid"}

	got, err := svc.UpdateLocale(context.Background(), viewer, "en")
	if err != nil {
		t.Fatalf("UpdateLocale: %v", err)
	}
	if got.Locale.String() != "en" {
		t.Errorf("Locale = %q, want en", got.Locale)
	}
	if store.updatedLocale != "en" {
		t.Errorf("store got %q, want en", store.updatedLocale)
	}
}
