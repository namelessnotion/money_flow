# frozen_string_literal: true
# typed: strict

require 'securerandom'
require 'faraday'
require_relative '../../gen/proto/holder/v1/holder_pb'
require_relative '../../gen/proto/holder/v1/holder_twirp'

module Services
  # onboards an entity into the system and provisions the necessary resources for it to operate
  class OnboardEntity < BaseService
    class ProvisioningFailed < StandardError; end

    MAX_ATTEMPTS = 3

    # Twirp codes worth another attempt: the call may never have reached the
    # service, or reached it and lost the reply. Provisioning is idempotent per
    # id, so retrying is safe. Anything else (invalid_argument, and the domain
    # rejection path) will fail identically however many times it is sent.
    RETRYABLE_CODES = T.let(%i[unavailable deadline_exceeded internal unknown].freeze, T::Array[Symbol])

    # A Wallet's uuid alongside the account type it backs, held together so the
    # same pair is sent to the Holder service and written locally.
    class WalletPlan < T::Struct
      const :type, Types::Enums::AccountType
      const :wallet_uuid, String
    end

    sig { params(holder_client: Holder::V1::HolderServiceClient).void }
    def initialize(
      holder_client: Holder::V1::HolderServiceClient.new(
        ENV.fetch('HOLDER_SERVICE_URL', 'http://localhost:8080/twirp')
      )
    )
      super()
      @holder_client = holder_client
    end

    sig { params(request: OnboardEntityRequest).returns(OnboardEntityResponse) }
    def call(request:)
      holder_uuid = SecureRandom.uuid_v7
      wallet_plans = plan_wallets

      # Provisioning happens before anything is written locally, and it is one
      # all-or-nothing call: the holder and every wallet exist in the event log,
      # or none of them do. Kept outside `perform` so a database transaction is
      # never held open across a network call. If the inserts below fail, the
      # provisioned holder and wallets are orphaned but inert — nothing can
      # reach uuids that were never persisted.
      provision!(holder_uuid, wallet_plans)

      perform { persist(request, holder_uuid, wallet_plans) }
    end

    private

    # One Wallet per account type, each with the uuid it will be opened under.
    sig { returns(T::Array[WalletPlan]) }
    def plan_wallets
      Types::Enums::AccountType.values.map do |type|
        WalletPlan.new(type: type, wallet_uuid: SecureRandom.uuid_v7)
      end
    end

    # Runs inside `perform`, so the entity and every account commit together or
    # not at all — there is no half-onboarded entity.
    sig do
      params(request: OnboardEntityRequest, holder_uuid: String, wallet_plans: T::Array[WalletPlan])
        .returns(OnboardEntityResponse)
    end
    def persist(request, holder_uuid, wallet_plans)
      entity = Models::Entity.create(name: request.name, holder_uuid: holder_uuid)

      wallet_plans.each do |plan|
        Models::Account.create(
          entity_id: entity.id,
          name: plan.type.serialize,
          type: plan.type.serialize,
          wallet_uuid: plan.wallet_uuid
        )
      end

      OnboardEntityResponse.new(success: true, entity_id: entity.id, holder_uuid: holder_uuid)
    end

    # Establishes the holder and opens one wallet per account type.
    #
    # Every attempt reuses the same uuids. Provision is idempotent per holder
    # and per wallet id, so a retry converges on the holder that may already
    # have been written — generating fresh uuids here is exactly what would turn
    # a transient timeout into permanent litter in the event log.
    sig { params(holder_uuid: String, wallet_plans: T::Array[WalletPlan]).void }
    def provision!(holder_uuid, wallet_plans)
      req = provision_request(holder_uuid, wallet_plans)

      attempt = 0
      loop do
        attempt += 1

        response = attempt_provision(req, attempt)
        next if response.nil? # transport failure with attempts left

        error = response.error
        return check_accepted!(response) if error.nil?

        raise ProvisioningFailed, error.msg unless retryable?(error, attempt)
      end
    end

    # Returns nil when a transport failure should be retried.
    #
    # A transport failure never becomes a Twirp::Error — the client's Faraday
    # call raises straight through. It is the same situation as a retryable
    # twirp code, though: the request may never have arrived, or arrived and
    # lost its reply, and provisioning is idempotent either way.
    sig { params(req: T.untyped, attempt: Integer).returns(T.untyped) }
    def attempt_provision(req, attempt)
      # #provision is defined via protoc-gen-twirp_ruby's `rpc` DSL at load time
      # (define_method), which Sorbet can't see statically — there's no RBI for
      # our own generated gen/proto code the way tapioca generates one for real
      # gems.
      T.unsafe(@holder_client).provision(req)
    rescue Faraday::Error => e
      raise ProvisioningFailed, e.message unless attempt < MAX_ATTEMPTS

      nil
    end

    # Returns a Holder::V1::ProvisionRequest, but that can't be named as a type:
    # the generated *_pb.rb build their classes by constant assignment from the
    # descriptor pool, so Sorbet sees a value rather than a class.
    sig { params(holder_uuid: String, wallet_plans: T::Array[WalletPlan]).returns(T.untyped) }
    def provision_request(holder_uuid, wallet_plans)
      Holder::V1::ProvisionRequest.new(
        id: holder_uuid,
        wallets: wallet_plans.map do |plan|
          Holder::V1::WalletSpec.new(
            wallet_id: plan.wallet_uuid,
            name: plan.type.serialize,
            allows: plan.type.allows
          )
        end
      )
    end

    # A domain refusal is a well-formed request the holder declined — a wallet
    # already belonging to someone else, say. It arrives as a successful
    # response, so it has to be unwrapped rather than trusted, and retrying it
    # cannot help.
    sig { params(response: Twirp::ClientResp).void }
    def check_accepted!(response)
      rejected = response.data&.holder_provision_rejected
      raise ProvisioningFailed, rejected.reason if rejected
    end

    sig { params(error: Twirp::Error, attempts: Integer).returns(T::Boolean) }
    def retryable?(error, attempts)
      attempts < MAX_ATTEMPTS && RETRYABLE_CODES.include?(error.code)
    end
  end
end
