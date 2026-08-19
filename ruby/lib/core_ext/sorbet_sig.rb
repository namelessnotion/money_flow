# frozen_string_literal: true
# typed: strict

require 'sorbet-runtime'

# Monkey patch: make `sig` available in every module/class without having to
# explicitly write `extend T::Sig` (or `include T::Sig`) in each file.
#
# Since `Module` is the ancestor of every class and module object, including
# `T::Sig` here makes its instance method `sig` available on all of them.
class Module
  include T::Sig
end
