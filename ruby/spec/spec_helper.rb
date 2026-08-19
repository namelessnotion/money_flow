# frozen_string_literal: true

require_relative '../lib/boot'

APP_ROOT = File.expand_path('..', __dir__)
%w[types models services].each do |layer|
  Dir[File.join(APP_ROOT, 'app', layer, '**', '*.rb')].each { |f| require f }
end

RSpec.configure do |config|
  config.expect_with :rspec do |expectations|
    expectations.include_chain_clauses_in_custom_matcher_descriptions = true
  end

  config.mock_with :rspec do |mocks|
    mocks.verify_partial_doubles = true
  end

  config.shared_context_metadata_behavior = :apply_to_host_groups

  config.around do |example|
    DB.transaction(rollback: :always) { example.run }
  end

  config.filter_run_when_matching :focus
  config.warnings = true
  config.order = :random
  Kernel.srand config.seed
end
