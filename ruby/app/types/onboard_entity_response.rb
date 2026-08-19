# frozen_string_literal: true
# typed: strict

# response type returned from onboarding an entity
class OnboardEntityResponse < T::Struct
  const :success, T::Boolean
  const :entity_id, Integer
  const :holder_uuid, String
end
