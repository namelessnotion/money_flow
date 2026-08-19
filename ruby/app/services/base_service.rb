# frozen_string_literal: true
# typed: strict

module Services
  # BaseService is a base class for all services in the system.
  # It provides common functionality and serves as a foundation for other service classes.
  class BaseService
    extend T::Sig

    sig { void }
    def initialize
      # Initialization logic for the base service
    end

    sig { params('&': T.proc.returns(T.untyped)).returns(T.untyped) }
    def perform(&)
      # savepoint: true makes this roll back on its own even when called
      # from inside a larger enclosing transaction (as in specs, or once
      # services start composing with each other), instead of only being
      # able to abort the whole outer transaction.
      DB.transaction(savepoint: true, &)
    end
  end
end
