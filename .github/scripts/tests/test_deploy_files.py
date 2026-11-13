"""The Docker deployment must not open private services to the network or
pin a published secret (deploy/docker-compose.yml, deploy/nginx.conf)."""

import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[3]
COMPOSE = (ROOT / "deploy" / "docker-compose.yml").read_text(encoding="utf-8")
NGINX = (ROOT / "deploy" / "nginx.conf").read_text(encoding="utf-8")


class DeployFiles(unittest.TestCase):
    def test_only_the_web_app_listens_on_the_network(self):
        lines = [line for line in COMPOSE.splitlines() if line.strip().startswith("ports:")]
        published = re.findall(r'"((?:[\d.]+:)?\d+:\d+)"', "\n".join(lines))
        self.assertIn("3000:80", published, "the web app is what other computers open")
        for port in published:
            if port == "3000:80":
                continue
            self.assertTrue(
                port.startswith("127.0.0.1:"),
                f"{port}: the database, the API and the mail catcher listen on this machine only",
            )

    def test_secrets_and_passwords_can_be_set_without_editing_the_file(self):
        for name in ("HP_ENV", "HP_AUTH_JWT_SECRET", "HP_FILES_URL_SECRET", "HP_MAIL_KEYS_SECRET", "HP_AI_KEYS_SECRET"):
            self.assertRegex(COMPOSE, rf"{name}: \$\{{{name}:-", f"{name} comes from deploy/.env when set")
        for line in COMPOSE.splitlines():
            if "postgres://" in line:
                self.assertRegex(line, r"://\w+:\$\{HP_DB_\w+_PASSWORD:-", "database passwords come from deploy/.env when set")
        self.assertIn("POSTGRES_PASSWORD: ${HP_DB_ADMIN_PASSWORD:-", COMPOSE)

    def test_the_api_docs_page_is_not_served_on_the_apps_address(self):
        self.assertIn("location = /docs { return 404; }", NGINX, "the docs page loads an outside script: only on the API's own port")
        self.assertIn("location = /openapi.json { return 404; }", NGINX)
        self.assertNotRegex(NGINX, r"location = /(docs|openapi\.json) \{ proxy_pass")
        self.assertIn("location /internal/ { return 404; }", NGINX)


if __name__ == "__main__":
    unittest.main()
