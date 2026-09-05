class GPAError(Exception):
    """User-facing CLI error."""

    def __init__(self, message: str, code: int = 1) -> None:
        super().__init__(message)
        self.code = code
