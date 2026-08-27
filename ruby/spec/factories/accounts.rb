# frozen_string_literal: true

FactoryBot.define do
  factory :account, class: 'Models::Account' do
    sequence(:name) { |n| "Account #{n}" }
    wallet_uuid { SecureRandom.uuid_v7 }
    type { 'bank' }
    entity

    to_create(&:save)
  end
end
