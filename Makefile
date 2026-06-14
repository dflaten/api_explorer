UV_RUN = uv run

.PHONY: test compat typecheck format check

test:
	$(UV_RUN) pytest

compat:
	$(UV_RUN) pytest tests/compat

typecheck:
	$(UV_RUN) ty check

format:
	$(UV_RUN) ruff format .

check: format typecheck test
