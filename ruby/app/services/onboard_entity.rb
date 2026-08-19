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
      perform do
        entity = Models::Entity.create(name: request.name)

        holder_uuid = SecureRandom.uuid
        # #establish is defined via protoc-gen-twirp_ruby's `rpc` DSL at load
        # time (define_method), which Sorbet can't see statically — there's
        # no RBI for our own generated gen/proto code the way tapioca
        # generates one for real gems.
        response = T.unsafe(@holder_client).establish(Holder::V1::EstablishRequest.new(id: holder_uuid))

        raise HolderEstablishmentFailed, response.error.msg if response.error

        entity.update(holder_uuid: holder_uuid)

        OnboardEntityResponse.new(success: true, entity_id: entity.id, holder_uuid: holder_uuid)
      end
    end
  end
end
