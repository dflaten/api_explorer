import json
import sys

import pytest
import yaml

import api_cli


def test_load_env_file_sets_missing_values_without_overriding_existing(
    tmp_path, monkeypatch
):
    env_path = tmp_path / ".env"
    env_path.write_text("API_TOKEN=from-dotenv\nexport MY_API_KEY=dotenv-key\nEMPTY=\n")

    monkeypatch.delenv("API_TOKEN", raising=False)
    monkeypatch.setenv("MY_API_KEY", "shell-value")

    api_cli.load_env_file(str(env_path))

    assert api_cli.os.environ["API_TOKEN"] == "from-dotenv"
    assert api_cli.os.environ["MY_API_KEY"] == "shell-value"
    assert api_cli.os.environ["EMPTY"] == ""


def test_discover_and_resolve_config_aliases(tmp_path):
    config_dir = tmp_path / "configs"
    config_dir.mkdir()
    alpha = config_dir / "alpha.yaml"
    beta = config_dir / "beta.yml"
    alpha.write_text("base_url: https://alpha.example.com\n")
    beta.write_text("base_url: https://beta.example.com\n")

    discovered = api_cli.discover_config_files(str(config_dir))

    assert discovered == [alpha, beta]
    assert api_cli.resolve_config_path("alpha", str(config_dir)) == str(alpha)
    assert api_cli.resolve_config_path("beta", str(config_dir)) == str(beta)


def test_write_config_template_creates_yaml_file(tmp_path):
    config_path = tmp_path / "demo.yaml"

    api_cli.write_config_template(str(config_path))

    loaded = yaml.safe_load(config_path.read_text())
    assert loaded["base_url"] == "https://api.example.com"
    assert "health" in loaded["endpoints"]


def test_update_env_value_replaces_existing_key(tmp_path):
    env_path = tmp_path / ".env"
    env_path.write_text("API_TOKEN=old-value\nOTHER=value\n")

    api_cli.update_env_value(str(env_path), "API_TOKEN", "new-token")

    assert env_path.read_text() == "API_TOKEN=new-token\nOTHER=value\n"


def test_persist_access_token_updates_env_from_config_placeholder(tmp_path):
    config_path = tmp_path / "demo.yaml"
    config_path.write_text(
        yaml.safe_dump(
            {
                "base_url": "https://api.example.com",
                "auth": {"type": "bearer", "token": "${API_TOKEN}"},
                "endpoints": {},
            },
            sort_keys=False,
        )
    )
    env_path = tmp_path / ".env"
    env_path.write_text("API_TOKEN=old-token\n")

    api_cli.persist_access_token(str(config_path), "new-token", str(env_path))

    assert env_path.read_text() == "API_TOKEN=new-token\n"
    updated_config = yaml.safe_load(config_path.read_text())
    assert updated_config["auth"]["token"] == "${API_TOKEN}"


def test_persist_access_token_keeps_multiple_api_tokens_separate(tmp_path):
    github_config = tmp_path / "github.yaml"
    github_config.write_text(
        yaml.safe_dump(
            {
                "auth": {"type": "bearer", "token": "${GITHUB_TOKEN}"},
                "endpoints": {},
            },
            sort_keys=False,
        )
    )
    slack_config = tmp_path / "slack.yaml"
    slack_config.write_text(
        yaml.safe_dump(
            {
                "auth": {"type": "bearer", "token": "${SLACK_TOKEN}"},
                "endpoints": {},
            },
            sort_keys=False,
        )
    )
    env_path = tmp_path / ".env"
    env_path.write_text("GITHUB_TOKEN=old-github\nSLACK_TOKEN=old-slack\n")

    api_cli.persist_access_token(str(github_config), "new-github-token", str(env_path))
    api_cli.persist_access_token(str(slack_config), "new-slack-token", str(env_path))

    assert env_path.read_text() == (
        "GITHUB_TOKEN=new-github-token\nSLACK_TOKEN=new-slack-token\n"
    )


def test_main_lists_configs(tmp_path, monkeypatch, capsys):
    config_dir = tmp_path / "configs"
    config_dir.mkdir()
    (config_dir / "github.yaml").write_text("base_url: https://api.github.com\n")

    monkeypatch.setattr(
        sys,
        "argv",
        ["api_cli.py", "--config-dir", str(config_dir), "--list-configs"],
    )

    api_cli.main()

    output = capsys.readouterr().out
    assert "Available configs:" in output
    assert "github" in output


def test_main_dry_run_uses_alias_config(tmp_path, monkeypatch, capsys):
    config_dir = tmp_path / "configs"
    config_dir.mkdir()
    config_path = config_dir / "github.yaml"
    config_path.write_text(
        yaml.safe_dump(
            {
                "base_url": "https://api.github.com",
                "default_headers": {"Authorization": "Bearer ${GITHUB_TOKEN}"},
                "endpoints": {
                    "get_repo": {
                        "method": "GET",
                        "path": "/repos/{owner}/{repo}",
                        "params": {"owner": "octocat", "repo": "Hello-World"},
                    }
                },
            },
            sort_keys=False,
        )
    )

    monkeypatch.setenv("GITHUB_TOKEN", "secret-token")
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "api_cli.py",
            "--config-dir",
            str(config_dir),
            "github",
            "get_repo",
            "--dry-run",
        ],
    )

    api_cli.main()

    output = capsys.readouterr().out
    assert "Request Preview:" in output
    assert "https://api.github.com/repos/octocat/Hello-World" in output
    assert "<redacted>" in output


def test_emit_response_body_formats_json_and_text(capsys):
    json_value, text_value = api_cli.emit_response_body({"ok": True})
    text_json = capsys.readouterr().out
    assert json_value == {"ok": True}
    assert text_value is None
    assert json.dumps({"ok": True}, indent=2) in text_json

    json_value, text_value = api_cli.emit_response_body("plain text")
    text_plain = capsys.readouterr().out
    assert json_value is None
    assert text_value == "plain text"
    assert "plain text" in text_plain
