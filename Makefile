UV_RUN = uv run

.PHONY: test typecheck format check

test:
	$(UV_RUN) pytest

typecheck:
	$(UV_RUN) ty check

format:
	$(UV_RUN) ruff format .

check: format typecheck test
