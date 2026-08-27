# frozen_string_literal: true
# typed: strict

module Connections
  # `GraphQL::Pagination::SequelDatasetConnection#load_nodes` (inherited from
  # `RelationConnection`) materializes the page via `Dataset#to_a`. Sequel never
  # defines `to_a`, so it falls back to `Enumerable#to_a`, which drives the
  # dataset through `#each` directly — skipping the eager-loading post-processing
  # that only runs inside `Dataset#all`. Left as-is, any connection field
  # returning a `.eager`-loaded dataset would silently degrade to one query per
  # row instead of the intended single batched query.
  class SequelDatasetConnection < GraphQL::Pagination::SequelDatasetConnection
    private

    # `@nodes` is owned by the gem superclass (`RelationConnection#nodes` reads
    # it directly), so it can't be renamed to `@load_nodes` — deliberately
    # written without `||=` so Rubocop's `Naming/MemoizedInstanceVariableName`
    # (which only fires on that memoization shorthand) doesn't apply here.
    #
    # `limited_nodes.all` is `T.untyped` (the gem superclass has no Sorbet
    # types of its own); `T.nilable` matches `@nodes`'s real lifecycle (unset
    # until the first call), and `T.must` on the freshly-assigned value is
    # safe because `Dataset#all` always returns an Array, never nil.
    sig { returns(T::Array[T.untyped]) }
    def load_nodes
      return @nodes if @nodes

      @nodes = T.let(limited_nodes.all, T.nilable(T::Array[T.untyped]))
      T.must(@nodes)
    end
  end
end
