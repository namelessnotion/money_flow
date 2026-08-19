# frozen_string_literal: true
# typed: ignore

Sequel.migration do
  change do
    create_table(:entities) do
      primary_key :id
      String :name, null: false
      String :holder_uuid, unique: true
      DateTime :created_at, null: false, default: Sequel::SQL::Constants::CURRENT_TIMESTAMP
      DateTime :updated_at, null: false, default: Sequel::SQL::Constants::CURRENT_TIMESTAMP
    end
  end
end
