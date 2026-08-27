# frozen_string_literal: true

require 'spec_helper'

RSpec.describe MoneyFlowSchema do
  let(:schema_path) { File.expand_path('money_flow_schema.graphql', "#{Environment::ROOT}/app/graphql/schemas") }

  describe 'the generated schema file' do
    it 'exists' do
      expect(File).to exist(schema_path)
    end

    it 'is current with the schema defined in code' do
      current_definition = "#{described_class.to_definition.strip}\n"

      expect(File.read(schema_path)).to eq(current_definition),
                                        'app/graphql/schemas/money_flow_schema.graphql is out of date. ' \
                                        'Run `rake graphql:schema:dump` to regenerate it.'
    end
  end
end
