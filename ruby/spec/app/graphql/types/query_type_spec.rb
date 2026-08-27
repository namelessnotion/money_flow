# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Types::QueryType do
  def execute(query, variables: {})
    MoneyFlowSchema.execute(query, variables: variables).to_h
  end

  # Records every SQL statement Sequel logs while the block runs, so tests can
  # assert on exactly which queries (and columns) were issued without stubbing
  # Sequel.
  def capture_sql
    statements = []
    recorder = Object.new
    %i[info warn error].each { |level| recorder.define_singleton_method(level) { |message| statements << message } }

    DB.loggers << recorder
    yield
    statements
  ensure
    DB.loggers.delete(recorder)
  end

  def create_entity(name:)
    create(:entity, name: name)
  end

  describe 'entities' do
    it 'returns onboarded entities' do
      entity = create_entity(name: 'Acme Corp')

      result = execute('{ entities { nodes { id name holderUuid } } }')

      expect(result['errors']).to be_nil
      expect(result.dig('data', 'entities', 'nodes')).to eq(
        [{ 'id' => entity.id.to_s, 'name' => 'Acme Corp', 'holderUuid' => entity.holder_uuid }]
      )
    end

    it 'paginates at 100 per page by default' do
      101.times { |n| create_entity(name: "Entity #{n}") }

      result = execute('{ entities { nodes { id } pageInfo { hasNextPage } } }')

      expect(result.dig('data', 'entities', 'nodes').length).to eq(100)
      expect(result.dig('data', 'entities', 'pageInfo', 'hasNextPage')).to be true
    end

    it 'honors an explicit first argument' do
      3.times { |n| create_entity(name: "Entity #{n}") }

      result = execute('{ entities(first: 2) { nodes { id } pageInfo { hasNextPage } } }')

      expect(result.dig('data', 'entities', 'nodes').length).to eq(2)
      expect(result.dig('data', 'entities', 'pageInfo', 'hasNextPage')).to be true
    end

    it 'selects only the requested entity columns' do
      create_entity(name: 'Acme Corp')

      sqls = capture_sql { execute('{ entities { nodes { id name } } }') }

      expect(sqls).to include(a_string_including('SELECT "id", "name" FROM "entities"'))
    end

    it 'eager loads accounts in a single query, without N+1s' do
      first = create_entity(name: 'Acme Corp')
      second = create_entity(name: 'Beta LLC')
      create(:account, name: 'Checking', type: 'bank', entity: first)
      create(:account, name: 'Savings', type: 'cash', entity: second)

      result = nil
      sqls = capture_sql do
        result = execute('{ entities { nodes { id accounts { name type } } } }')
      end

      expect(result['errors']).to be_nil
      expect(sqls.count { |sql| sql.include?('FROM "accounts"') }).to eq(1)

      accounts_by_entity = result.dig('data', 'entities', 'nodes').to_h do |node|
        [node['id'], node['accounts'].map { |account| account['name'] }]
      end
      expect(accounts_by_entity).to eq(
        first.id.to_s => ['Checking'],
        second.id.to_s => ['Savings']
      )
    end

    it 'does not eager load accounts when they are not requested' do
      create_entity(name: 'Acme Corp')

      sqls = capture_sql { execute('{ entities { nodes { id } } }') }

      expect(sqls.none? { |sql| sql.include?('FROM "accounts"') }).to be true
    end
  end
end
