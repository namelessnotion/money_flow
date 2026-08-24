# typed: false
# frozen_string_literal: true

# `tapioca dsl` has no way to boot a non-Rails app: `Tapioca::Loaders::Dsl#load`
# only calls `load_rails_application`, which returns immediately when there's no
# `config/application.rb`, and `dsl` has no `--require` flag.
#
# This runs from the extensions glob, which `Tapioca::Loaders::Dsl#load` requires
# *before* both the (no-op) application load and the compilers. That ordering
# matters beyond our own compiler: tapioca's built-in compilers are guarded by
# things like `return unless defined?(Google::Protobuf)`, so an app loaded any
# later would leave them silently disabled.
#
# Loading the app needs a live, migrated database — defining a Sequel::Model
# subclass issues a real schema query.
require_relative '../../../lib/environment'
