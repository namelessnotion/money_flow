# frozen_string_literal: true
# typed: strict

require_relative 'objects/base_object'

module Types
  # Placeholder root Query type. GraphQL schemas require a query root to be
  # valid even before there's anything real to query — replace `ok` with
  # real fields as read models come online.
  class QueryType < BaseObject
    field :ok, Boolean, null: false, resolver_method: :ok?,
                        description: 'Health check placeholder until real queries exist.'

    sig { returns(T::Boolean) }
    def ok?
      true
    end
  end
end
