# frozen_string_literal: true
# typed: ignore

Sequel.migration do
  change do
    create_table(:accounts) do
      primary_key :id
      String :name, null: false
      String :wallet_uuid, null: false, unique: true
      String :type, null: false
      DateTime :created_at, null: false, default: Sequel::SQL::Constants::CURRENT_TIMESTAMP
      DateTime :updated_at, null: false, default: Sequel::SQL::Constants::CURRENT_TIMESTAMP
    end
  end
end
