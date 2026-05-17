"""Domain-specific exceptions for the extraction pipeline."""


class ExtractionError(Exception):
    """Raised when an extraction engine fails irrecoverably."""


class ValidationError(Exception):
    """Raised when extracted data fails a validation check.

    Attributes:
        failures: List of human-readable validation failure descriptions.
    """

    def __init__(self, failures: list[str]) -> None:
        self.failures = failures
        super().__init__(f"Validation failed: {', '.join(failures)}")


class TemplateMatchError(ExtractionError):
    """Raised when invoice2data cannot match any template."""


class LLMExtractionError(ExtractionError):
    """Raised when the LLM fails to produce a valid extraction."""


class TemplateGenerationError(Exception):
    """Raised when template generation cannot complete.

    Attributes:
        retryable: True for transient failures (retry up to MAX_RETRIES).
                   False for permanent failures (DLQ immediately).
        reason: Short outcome tag matching the done-event log.
    """

    def __init__(self, reason: str, *, retryable: bool) -> None:
        self.retryable = retryable
        self.reason = reason
        super().__init__(f"Template generation failed ({reason}), retryable={retryable}")
