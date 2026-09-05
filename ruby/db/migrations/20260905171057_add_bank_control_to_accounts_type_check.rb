# frozen_string_literal: true
# typed: ignore

# Values written out literally, matching the constraint this replaces
# (20260822211706_add_accounts_type_check.rb) — a migration has to keep
# meaning what it meant when it ran, and would otherwise silently change
# whenever Types::Enums::AccountType grows.
OLD_ACCOUNT_TYPES = %w[
  bank
  debit_card
  uncleared_cash
  cleared_cash
  cash
  gain
  loss
].freeze

NEW_ACCOUNT_TYPES = %w[
  bank
  bank_control
  debit_card
  uncleared_cash
  cleared_cash
  cash
  gain
  loss
].freeze

Sequel.migration do
  up do
    alter_table(:accounts) do
      drop_constraint(:accounts_type_check)
      add_constraint(:accounts_type_check, type: NEW_ACCOUNT_TYPES)
    end
  end

  down do
    alter_table(:accounts) do
      drop_constraint(:accounts_type_check)
      add_constraint(:accounts_type_check, type: OLD_ACCOUNT_TYPES)
    end
  end
end
