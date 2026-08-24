# frozen_string_literal: true
# typed: ignore

# The values are written out rather than read from Types::Enums::AccountType on
# purpose: a migration has to keep meaning what it meant when it ran, and would
# otherwise silently change shape whenever the enum grows.
#
# A CHECK rather than a pg enum type — AccountType will gain values, and
# `ALTER TYPE ... ADD VALUE` has transaction restrictions that a
# drop-and-recreate of a CHECK does not.
#
# Deliberately NOT unique on (entity_id, type): an entity may hold several Bank
# accounts. `wallet_uuid UNIQUE` is the invariant that actually matters — one
# account per Wallet.
ACCOUNT_TYPES = %w[
  bank
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
      add_constraint(:accounts_type_check, type: ACCOUNT_TYPES)
    end
  end

  down do
    alter_table(:accounts) do
      drop_constraint(:accounts_type_check)
    end
  end
end
