# frozen_string_literal: true
# typed: strict

require_relative 'base_object'

module Types
  # GraphQL type for the Entity model
  class EntityType < BaseObject
    field :id, ID, null: false
    field :name, String, null: false
    field :holder_uuid, ID, null: false
    field :created_at, GraphQL::Types::ISO8601DateTime, null: false
    field :updated_at, GraphQL::Types::ISO8601DateTime, null: false
  end
end
