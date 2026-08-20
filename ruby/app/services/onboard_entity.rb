# frozen_string_literal: true
# typed: strict

require 'securerandom'
require_relative '../../gen/proto/holder/v1/holder_pb'
require_relative '../../gen/proto/holder/v1/holder_twirp'

module Services
  # onboards an entity into the system and provisions the necessary resources for it to operate
  class OnboardEntity < BaseService
    class HolderEstablishmentFailed < StandardError; end

    sig { params(holder_client: Holder::V1::HolderServiceClient).void }
    def initialize(holder_client: Holder::V1::HolderServiceClient.new(ENV.fetch('HOLDER_SERVICE_URL', 'http://localhost:8080')))
      super()
      @holder_client = holder_client
    end

    sig { params(request: OnboardEntityRequest).returns(OnboardEntityResponse) }
    def call(request:)
      holder_uuid = SecureRandom.uuid

      # The holder is established before the row is written so the entity can be
      # inserted with its holder_uuid already set — entities.holder_uuid is NOT
      # NULL, and there is no valid state where an entity exists without a
      # holder. Kept outside `perform` so a database transaction is never held
      # open across a network call; if the insert below fails, the established
      # holder is orphaned but inert, since nothing can reach a uuid that was
      # never persisted.
      #
      # #establish is defined via protoc-gen-twirp_ruby's `rpc` DSL at load
      # time (define_method), which Sorbet can't see statically — there's
      # no RBI for our own generated gen/proto code the way tapioca
      # generates one for real gems.
      response = T.unsafe(@holder_client).establish(Holder::V1::EstablishRequest.new(id: holder_uuid))

      raise HolderEstablishmentFailed, response.error.msg if response.error

      perform do
        entity = Models::Entity.create(name: request.name, holder_uuid: holder_uuid)

        OnboardEntityResponse.new(success: true, entity_id: entity.id, holder_uuid: holder_uuid)
      end
    end
  end
end
