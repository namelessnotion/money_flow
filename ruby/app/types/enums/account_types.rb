# frozen_string_literal: true
# typed: strict

module Types
  module Enums
    # Different types of accounts that can be created.
    #
    # Every value states its serialized form explicitly. T::Enum would otherwise
    # derive it by downcasing the constant with no separator ("debitcard",
    # "unclearedcash"), and these strings are persisted in accounts.type and
    # sent over the wire as a Wallet's name.
    class AccountType < T::Enum
      extend T::Sig

      enums do
        Bank = new('bank')                     # ACH Bank Account
        DebitCard = new('debit_card')          # Debit Card Account
        UnclearedCash = new('uncleared_cash')  # Cash waiting to be cleared
        ClearedCash = new('cleared_cash')      # Cash that has cleared
        Cash = new('cash')                     # Cash that can be used for purchases
        Gain = new('gain')                     # Gain account for tracking profits
        Loss = new('loss')                     # Loss account for tracking losses
      end

      # What the backing Wallet may do at the platform boundary: onramp brings
      # money in from outside, offramp sends it out. Only funding instruments
      # touch that boundary — everything else moves money that is already
      # inside the platform, so it permits neither.
      #
      # ALLOWS_NONE rather than ALLOWS_UNSPECIFIED: the Wallet service rejects
      # an unset policy, so "neither direction" has to be said out loud.
      #
      # Returned as the enum's symbol rather than its integer. google-protobuf
      # accepts either and raises RangeError on an unknown name, whereas the
      # generated Shared::V1::Allows::* constants are built at runtime from the
      # descriptor pool and so cannot be resolved statically.
      sig { returns(Symbol) }
      def allows
        case self
        when Bank then :ALLOWS_ONRAMP_AND_OFFRAMP
        when DebitCard then :ALLOWS_ONRAMP
        else :ALLOWS_NONE
        end
      end
    end
  end
end
