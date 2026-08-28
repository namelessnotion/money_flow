# frozen_string_literal: true
# typed: ignore

Sequel.migration do
  up do
    run 'ALTER TABLE accounts ALTER COLUMN wallet_uuid TYPE uuid USING wallet_uuid::uuid'
  end

  down do
    run 'ALTER TABLE accounts ALTER COLUMN wallet_uuid TYPE text'
  end
end
