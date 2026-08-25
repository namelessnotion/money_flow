# frozen_string_literal: true
# typed: strict

# Entry point for `rackup` / `rackup -p 9292` (9292 is rack's own default, so
# no explicit port config is needed to satisfy that requirement).
require_relative 'app/app'

run App.new
