// Package ledger wraps TigerBeetle: the source of truth for account
// balances, as opposed to eventstore which is the source of truth for
// intent. Token and Operation ids are used directly as TigerBeetle's
// 128-bit account/transfer IDs, so callers into this package must pass
// valid UUIDs.
package ledger

import "context"

// AccountFlags mirrors the subset of TigerBeetle account flags this domain
// needs. Linked chains this account's creation to the next one in the same
// CreateAccounts call so they succeed or fail together; it must be false on
// the last account of any batch. CreditsMustNotExceedDebits and
// DebitsMustNotExceedCredits are mutually exclusive and are set at Token
// mint time from the owning Wallet's Allows policy.
type AccountFlags struct {
	Linked                     bool
	CreditsMustNotExceedDebits bool
	DebitsMustNotExceedCredits bool
}

// Account is a TigerBeetle account to create, backing one Token.
type Account struct {
	ID       string // valid UUID; used as the TigerBeetle account id
	Currency string // resolved to TigerBeetle's numeric ledger via currency.go
	Flags    AccountFlags
}

// AccountResultCode is the outcome of creating one Account.
type AccountResultCode int

const (
	AccountResultUnspecified AccountResultCode = iota
	// AccountResultOK means the account was created.
	AccountResultOK
	// AccountResultExists means an account with this id already exists with
	// identical fields — treated as success by callers (idempotent retry).
	AccountResultExists
	// AccountResultExistsWithDifferentFlags means an account with this id
	// already exists but with different fields — a real conflict, not a
	// safe retry.
	AccountResultExistsWithDifferentFlags
	// AccountResultLinkedEventFailed means this account was part of a
	// flags.Linked chain and a different entry in the chain failed, so this
	// one was not created either.
	AccountResultLinkedEventFailed
	// AccountResultInvalid means the account's own fields are malformed
	// (e.g. an unrecognized currency) — it was rejected on its own merits,
	// not because of a sibling in a linked chain.
	AccountResultInvalid
)

// AccountResult reports the outcome of creating one Account, at the same
// index it was submitted at.
type AccountResult struct {
	Index  int
	Result AccountResultCode
}

// TransferKind selects which of TigerBeetle's transfer semantics this
// Transfer uses. Regular posts immediately. Pending reserves capacity
// (debits_pending/credits_pending) without posting it — used for a staged
// Transfer's initial DEBIT batch. PostPending finalizes a prior Pending
// transfer into posted balance. VoidPending releases a prior Pending
// transfer's reservation without posting anything.
type TransferKind int

const (
	TransferKindUnspecified TransferKind = iota
	TransferKindRegular
	TransferKindPending
	TransferKindPostPending
	TransferKindVoidPending
)

// Transfer is a TigerBeetle transfer to submit, one leg of an Operation.
type Transfer struct {
	ID              string // valid UUID; used as the TigerBeetle transfer id
	DebitAccountID  string
	CreditAccountID string
	MinorUnits      uint64
	Currency        string
	Linked          bool
	Kind            TransferKind

	// PendingID is required for TransferKindPostPending and
	// TransferKindVoidPending: the id of the TransferKindPending transfer
	// being finalized or released. Empty for other kinds.
	PendingID string

	// Timeout is how long a TransferKindPending reservation is held before
	// TigerBeetle auto-voids it, if it is never posted or voided first.
	// Zero means no timeout. Ignored for other kinds.
	Timeout int64 // seconds
}

// TransferResultCode is the outcome of submitting one Transfer.
type TransferResultCode int

const (
	TransferResultUnspecified TransferResultCode = iota
	// TransferResultOK means the transfer was applied (posted, reserved,
	// finalized, or voided, depending on Kind).
	TransferResultOK
	// TransferResultExists means a transfer with this id already exists
	// with identical fields — treated as success by callers.
	TransferResultExists
	// TransferResultExistsWithDifferentFields means a transfer with this id
	// already exists but with different fields — a real conflict.
	TransferResultExistsWithDifferentFields
	// TransferResultExceedsCredits means the debit account's
	// debits_must_not_exceed_credits flag would be violated.
	TransferResultExceedsCredits
	// TransferResultExceedsDebits means the credit account's
	// credits_must_not_exceed_debits flag would be violated.
	TransferResultExceedsDebits
	// TransferResultLinkedEventFailed means this transfer was part of a
	// flags.Linked chain and a different entry in the chain failed.
	TransferResultLinkedEventFailed
	// TransferResultPendingTransferNotFound means Kind was PostPending or
	// VoidPending but PendingID does not name a live pending transfer.
	TransferResultPendingTransferNotFound
	// TransferResultAccountNotFound means DebitAccountID or CreditAccountID
	// does not name an existing account.
	TransferResultAccountNotFound
	// TransferResultInvalid means the transfer's own fields are malformed
	// (e.g. a zero amount, mismatched ledgers, a missing required field) —
	// rejected on its own merits, not because of a sibling in a linked
	// chain and not one of the specific outcomes above.
	TransferResultInvalid
)

// TransferResult reports the outcome of submitting one Transfer, at the
// same index it was submitted at.
type TransferResult struct {
	Index  int
	Result TransferResultCode
}

// Client is TigerBeetle's boundary: the two batch-submission calls every
// mint and every Transfer saga step goes through, plus a balance read.
// Implementations must be safe for concurrent use.
type Client interface {
	// CreateAccounts submits a batch of account-creation requests. Entries
	// with Flags.Linked set are chained to the following entry so the whole
	// run of linked entries succeeds or fails together; the last entry in
	// any linked chain must not set Flags.Linked.
	CreateAccounts(ctx context.Context, accounts []Account) ([]AccountResult, error)

	// CreateTransfers submits a batch of transfers. Entries with Linked set
	// are chained the same way as CreateAccounts.
	CreateTransfers(ctx context.Context, transfers []Transfer) ([]TransferResult, error)

	// AccountBalance returns the given account's net balance — posted
	// credits minus posted debits, not including any pending reservation —
	// or found=false if no such account exists. Signed because an account
	// with neither flag set (free to move in either direction) can go
	// negative.
	AccountBalance(ctx context.Context, accountID string) (minorUnits int64, found bool, err error)
}
