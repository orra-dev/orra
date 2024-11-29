#  This Source Code Form is subject to the terms of the Mozilla Public
#   License, v. 2.0. If a copy of the MPL was not distributed with this
#   file, You can obtain one at https://mozilla.org/MPL/2.0/.

class OrraError(Exception):
    """Base exception for all Orra SDK errors"""
    def __init__(self, message: str, details: dict = None):
        super().__init__(message)
        self.message = message
        self.details = details or {}

class PersistenceError(OrraError):
    """Raised when there's an error with service key persistence"""
    pass

class ConnectionError(OrraError):
    """Raised when there's an error with WebSocket connection"""
    pass

class ServiceRegistrationError(OrraError):
    """Raised when service registration fails"""
    pass
