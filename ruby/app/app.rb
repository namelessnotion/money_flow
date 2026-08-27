# frozen_string_literal: true
# typed: strict

require 'json'
require 'rack'
require_relative '../lib/environment'

# `POST /graphql`, plus a `GET /healthz` liveness check.
class App
  extend T::Sig

  sig { params(env: T::Hash[String, T.untyped]).returns(T::Array[T.untyped]) }
  def call(env)
    request = Rack::Request.new(env)
    status, headers, body = route(request)

    [status, headers.merge(cors_headers(request)), body]
  end

  private

  sig { params(request: Rack::Request).returns(T::Array[T.untyped]) }
  def route(request)
    case [request.request_method, request.path]
    in ['OPTIONS', '/graphql']
      preflight
    in ['GET', '/healthz']
      healthz
    in ['POST', '/graphql']
      graphql(request)
    else
      not_found
    end
  end

  # Only the client origin needs cross-origin access to the GraphQL API;
  # everything else (healthz, twirp) is same-origin or server-to-server.
  sig { returns(String) }
  def allowed_origin
    ENV.fetch('CORS_ALLOWED_ORIGIN', 'https://app.local.namelessnotion.com')
  end

  sig { params(request: Rack::Request).returns(T::Hash[String, String]) }
  def cors_headers(request)
    origin = request.get_header('HTTP_ORIGIN')
    return {} unless origin == allowed_origin

    { 'access-control-allow-origin' => origin, 'vary' => 'Origin' }
  end

  sig { returns(T::Array[T.untyped]) }
  def preflight
    [204, { 'access-control-allow-methods' => 'POST, OPTIONS', 'access-control-allow-headers' => 'Content-Type' }, []]
  end

  sig { returns(T::Array[T.untyped]) }
  def healthz
    json_response(200, status: 'ok')
  end

  sig { params(request: Rack::Request).returns(T::Array[T.untyped]) }
  def graphql(request)
    payload = JSON.parse(request.body.read)

    if payload.is_a?(Array)
      multiplexed(payload)
    else
      single(payload)
    end
  rescue JSON::ParserError => e
    json_response(400, errors: [{ message: "invalid JSON: #{e.message}" }])
  end

  sig { params(payload: T::Hash[String, T.untyped]).returns(T::Array[T.untyped]) }
  def single(payload)
    result = MoneyFlowSchema.execute(
      payload['query'],
      variables: payload['variables'] || {},
      operation_name: payload['operationName'],
      context: {}
    )

    json_response(200, result.to_h)
  end

  # Multiplexed requests are a JSON array of query objects, each executed
  # independently but batched into a single response array, e.g.
  # `[{ "query" => "...", "variables" => {} }, { "query" => "..." }]`.
  sig { params(payloads: T::Array[T.untyped]).returns(T::Array[T.untyped]) }
  def multiplexed(payloads)
    queries = payloads.map do |payload|
      {
        query: payload['query'],
        variables: payload['variables'] || {},
        operation_name: payload['operationName']
      }
    end

    results = MoneyFlowSchema.multiplex(queries, context: {})

    json_response(200, results.map(&:to_h))
  end

  sig { returns(T::Array[T.untyped]) }
  def not_found
    json_response(404, errors: [{ message: 'not found' }])
  end

  sig { params(status: Integer, body: T.untyped).returns(T::Array[T.untyped]) }
  def json_response(status, body)
    [status, { 'content-type' => 'application/json' }, [JSON.generate(body)]]
  end
end
