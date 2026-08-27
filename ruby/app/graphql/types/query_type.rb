# frozen_string_literal: true
# typed: strict

require_relative 'objects/base_object'
require_relative 'objects/entity'

module Types
  # Root Query type.
  class QueryType < BaseObject
    field :ok, Boolean, null: false, resolver_method: :ok?,
                        description: 'Health check placeholder until real queries exist.'

    field :entities, Types::Entity.connection_type, null: false,
                                                    default_page_size: 100, max_page_size: 100,
                                                    extras: [:lookahead],
                                                    description: 'Onboarded entities, paginated at 100 per page.'

    sig { returns(T::Boolean) }
    def ok?
      true
    end

    # `Models::Entity::PrivateDataset` is a Sorbet-only fiction (see
    # `Types::Entity.scope`) so this uses `T::Sig::WithoutRuntime.sig` rather
    # than a normal `sig`, which would try to resolve the constant at runtime.
    T::Sig::WithoutRuntime.sig do
      params(lookahead: GraphQL::Execution::Lookahead).returns(Models::Entity::PrivateDataset)
    end
    def entities(lookahead:)
      Types::Entity.scope(lookahead, Models::Entity.dataset.order(:id))
    end
  end
end
