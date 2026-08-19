# frozen_string_literal: true
# typed: strict

# type to be passed to onboard an entity
class OnboardEntityRequest < T::Struct
  const :name, String
end
