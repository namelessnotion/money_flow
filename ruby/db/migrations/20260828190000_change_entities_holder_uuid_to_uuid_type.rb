# frozen_string_literal: true
# typed: ignore

Sequel.migration do
  up do
    run 'ALTER TABLE entities ALTER COLUMN holder_uuid TYPE uuid USING holder_uuid::uuid'
  end

  down do
    run 'ALTER TABLE entities ALTER COLUMN holder_uuid TYPE text'
  end
end
