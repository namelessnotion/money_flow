# frozen_string_literal: true
# typed: strict

require_relative '../types/query_type'
require_relative '../types/mutation_type'

# GraphQL schema for the MoneyFlow application
class MoneyFlow < GraphQL::Schema
  query Types::QueryType
  mutation Types::MutationType
end
