# frozen_string_literal: true
# typed: ignore

Sequel.migration do
  change do
    alter_table(:accounts) do
      add_foreign_key :entity_id, :entities, index: true, null: false
    end
  end
end
