# frozen_string_literal: true
# typed: strict

module Models
  # Repsents a financials account held by an entity. This is a base class for all account types.
  class Account < Sequel::Model
    many_to_one :entity
  end
end
