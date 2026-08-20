# frozen_string_literal: true
# typed: false

require 'sequel'
require 'graphql'
require_relative 'core_ext/sorbet_sig'

DB = Sequel.connect(
  ENV.fetch('DATABASE_URL', 'postgres://money_flow:money_flow@localhost:5432/money_flow_dev?sslmode=disable')
)

# Generated protobuf/twirp code (e.g. `require 'holder/v1/holder_pb'`) lives
# under gen/proto rather than lib, so it isn't on the load path by default.
$LOAD_PATH.unshift(File.expand_path('../gen/proto', __dir__))
