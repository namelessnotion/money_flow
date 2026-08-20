# frozen_string_literal: true
# typed: strict

require_relative 'objects/base_object'
require_relative '../mutations/onboard_entity'

module Types
  # Root Mutation type
  class MutationType < BaseObject
    field :onboard_entity, mutation: Mutations::OnboardEntity
  end
end
