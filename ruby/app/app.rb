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

    case [request.request_method, request.path]
    in ['GET', '/healthz']
      healthz
    in ['POST', '/graphql']
      graphql(request)
    else
      not_found
    end
  end

  private

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
    result = MoneyFlow.execute(
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

    results = MoneyFlow.multiplex(queries, context: {})

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
