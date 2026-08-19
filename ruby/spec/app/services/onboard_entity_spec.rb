# frozen_string_literal: true

require 'spec_helper'

RSpec.describe Services::OnboardEntity do
  # A real client, never actually connected to over the network in these
  # specs — just a concrete instance of the sig-required type so we can
  # stub #establish on it (an instance_double won't satisfy the sig, since
  # sorbet-runtime checks the argument via is_a?).
  subject(:service) { described_class.new(holder_client: holder_client) }

  let(:holder_client) { Holder::V1::HolderServiceClient.new('http://holder.internal.test') }
  let(:request) { OnboardEntityRequest.new(name: 'Test Entity') }

  describe '#call' do
    context 'when the holder is established successfully' do
      before do
        allow(holder_client).to receive(:establish).and_return(
          Twirp::ClientResp.new(data: Holder::V1::EstablishResponse.new)
        )
      end

      it 'creates the entity and persists the holder uuid returned by EstablishHolder' do
        response = service.call(request: request)

        expect(response).to be_a(OnboardEntityResponse)
        expect(response.success).to be true

        entity = Models::Entity[response.entity_id]
        expect(entity.name).to eq('Test Entity')
        expect(entity.holder_uuid).to eq(response.holder_uuid)
      end

      it 'sends the newly generated holder id to EstablishHolder' do
        service.call(request: request)

        expect(holder_client).to have_received(:establish) do |establish_request|
          expect(establish_request.id).not_to be_empty
        end
      end
    end

    context 'when establishing the holder fails' do
      before do
        allow(holder_client).to receive(:establish).and_return(
          Twirp::ClientResp.new(error: Twirp::Error.internal('boom'))
        )
      end

      it 'raises and rolls back the entity creation' do
        expect { service.call(request: request) }.to raise_error(described_class::HolderEstablishmentFailed, 'boom')

        expect(Models::Entity.where(name: 'Test Entity')).to be_empty
      end
    end
  end
end
