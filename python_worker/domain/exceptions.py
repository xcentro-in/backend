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
