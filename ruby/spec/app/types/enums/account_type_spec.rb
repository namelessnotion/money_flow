# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Types::Enums::AccountType do
  it 'serializes every value with word separators' do
    # The T::Enum default would give "debitcard"/"unclearedcash", and these
    # strings are persisted in accounts.type and sent as a Wallet's name.
    expect(described_class.values.map(&:serialize)).to contain_exactly(
      'bank', 'bank_control', 'debit_card', 'uncleared_cash', 'cleared_cash', 'cash', 'gain', 'loss'
    )
  end

  describe '#allows' do
    it 'lets only funding instruments cross the platform boundary' do
      expect(described_class::Bank.allows).to eq(:ALLOWS_ONRAMP_AND_OFFRAMP)
      expect(described_class::BankControl.allows).to eq(:ALLOWS_ONRAMP_AND_OFFRAMP)
      expect(described_class::DebitCard.allows).to eq(:ALLOWS_ONRAMP)
    end

    it 'permits neither direction for everything else' do
      internal = described_class.values - [
        described_class::Bank, described_class::BankControl, described_class::DebitCard
      ]
      expect(internal.map(&:allows).uniq).to eq([:ALLOWS_NONE])
    end

    it 'never returns the unspecified value' do
      # The Wallet service rejects an unset policy, so this would fail onboarding.
      expect(described_class.values.map(&:allows)).not_to include(:ALLOWS_UNSPECIFIED)
    end

    # The symbols aren't checked statically, so a typo would only surface when a
    # real request was built. google-protobuf raises RangeError on an unknown
    # enum name, which makes this a real guard rather than a restatement.
    it 'returns a symbol protobuf accepts for every value' do
      types = described_class.values
      types.each do |type|
        expect { Holder::V1::WalletSpec.new(wallet_id: 'w', name: type.serialize, allows: type.allows) }
          .not_to raise_error
      end
    end
  end
end
