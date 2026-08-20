# frozen_string_literal: true
# typed: ignore

Sequel.migration do
  up do
    alter_table(:entities) do
      set_column_not_null :holder_uuid
    end
  end

  down do
    alter_table(:entities) do
      set_column_allow_null :holder_uuid
    end
  end
end
