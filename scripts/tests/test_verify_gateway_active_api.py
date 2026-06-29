import json
import tempfile
import textwrap
import unittest
from pathlib import Path

from scripts.verify_gateway_active_api import verify_contract


VALID_OPENAPI = textwrap.dedent(
    """
    openapi: 3.1.0
    info:
      title: Gateway
      version: 0.1.0
    security:
      - bearerAuth: []
    components:
      securitySchemes:
        bearerAuth:
          type: http
          scheme: bearer
    paths:
      /healthz:
        get:
          tags: [health]
          operationId: getHealthz
          security: []
          responses:
            "200":
              description: OK
      /readyz:
        get:
          tags: [health]
          operationId: getReadyz
          security: []
          responses:
            "200":
              description: OK
      /api/v1/users:
        post:
          tags: [auth]
          operationId: createUser
          security: []
          x-owner-service: auth
          responses:
            "201":
              description: Created
            "400":
              description: Bad request
      /api/v1/knowledge-queries:
        post:
          tags: [knowledge]
          operationId: createKnowledgeQuery
          x-owner-service: knowledge
          responses:
            "200":
              description: OK
            "400":
              description: Bad request
    x-missing-contracts:
      - service: gateway
        status: missing
        reason: Management overview fields are not finalized.
        placeholderOperations:
          - GET /api/v1/admin-overview
    """
)


VALID_OWNER_MAP = textwrap.dedent(
    """
    # Gateway Active API Owner Map

    ## Audit Result

    - Active operations: `4`.

    ## Owner Summary

    | Owner service | Active operations | Notes |
    | --- | ---: | --- |
    | `gateway` | 2 | Gateway health/readiness. |
    | `auth` | 1 | Auth surface. |
    | `knowledge` | 1 | Knowledge queries. |

    ## Missing Contracts

    | Placeholder operation | Expected owner | Status | Frontend/backend rule |
    | --- | --- | --- | --- |
    | `GET /api/v1/admin-overview` | `gateway` aggregation | missing | Do not generate frontend client methods. |

    ## Active Operations

    | Method | Path | Owner service | Tag | Operation ID | Auth |
    | --- | --- | --- | --- | --- | --- |
    | `GET` | `/healthz` | `gateway` | `health` | `getHealthz` | `none` |
    | `GET` | `/readyz` | `gateway` | `health` | `getReadyz` | `none` |
    | `POST` | `/api/v1/users` | `auth` | `auth` | `createUser` | `none` |
    | `POST` | `/api/v1/knowledge-queries` | `knowledge` | `knowledge` | `createKnowledgeQuery` | `bearerAuth` |
    """
)


VALID_WEB_PACKAGE = {
    "scripts": {
        "api:generate": "openapi-typescript ../../docs/services/gateway/api/openapi.yaml -o src/api/generated/gateway.ts"
    }
}


class GatewayActiveAPIContractTests(unittest.TestCase):
    def test_valid_contract_has_no_issues(self) -> None:
        issues = self.verify()

        self.assertEqual([], issues)

    def test_missing_required_api_operation_fields_fail(self) -> None:
        openapi = VALID_OPENAPI.replace("      operationId: createKnowledgeQuery\n", "")

        issues = self.verify(openapi=openapi)

        self.assertIssueContains(issues, "POST /api/v1/knowledge-queries missing operationId")

    def test_missing_owner_and_effective_security_fail(self) -> None:
        openapi = VALID_OPENAPI.replace("security:\n  - bearerAuth: []\n", "", 1).replace(
            "      x-owner-service: knowledge\n",
            "",
        )

        issues = self.verify(openapi=openapi)

        self.assertIssueContains(issues, "POST /api/v1/knowledge-queries missing x-owner-service")
        self.assertIssueContains(issues, "POST /api/v1/knowledge-queries missing security")

    def test_missing_4xx_response_on_active_api_operation_fails(self) -> None:
        openapi = VALID_OPENAPI.replace(
            '        "400":\n          description: Bad request\n',
            "",
            1,
        )

        issues = self.verify(openapi=openapi)

        self.assertIssueContains(issues, "POST /api/v1/users missing 4XX response")

    def test_action_style_active_path_segment_fails(self) -> None:
        openapi = VALID_OPENAPI.replace("/api/v1/knowledge-queries:", "/api/v1/search:")

        issues = self.verify(openapi=openapi)

        self.assertIssueContains(issues, "/api/v1/search uses action-style segment `search`")

    def test_missing_contract_placeholder_cannot_overlap_active_operation(self) -> None:
        openapi = VALID_OPENAPI.replace(
            "      - GET /api/v1/admin-overview",
            "      - POST /api/v1/knowledge-queries",
        )

        issues = self.verify(openapi=openapi)

        self.assertIssueContains(
            issues,
            "x-missing-contracts placeholder POST /api/v1/knowledge-queries overlaps active paths",
        )

    def test_frontend_generation_must_use_gateway_openapi_source(self) -> None:
        web_package = {
            "scripts": {
                "api:generate": "openapi-typescript ../../docs/services/ai-gateway/api/openapi.yaml -o src/api/generated/gateway.ts"
            }
        }

        issues = self.verify(web_package=web_package)

        self.assertIssueContains(issues, "apps/web api:generate must use ../../docs/services/gateway/api/openapi.yaml")

    def test_owner_map_drift_fails(self) -> None:
        owner_map = VALID_OWNER_MAP.replace(
            "| `POST` | `/api/v1/knowledge-queries` | `knowledge` |",
            "| `POST` | `/api/v1/knowledge-queries` | `document` |",
        )

        issues = self.verify(owner_map=owner_map)

        self.assertIssueContains(issues, "owner map active operation table differs from OpenAPI")

    def verify(
        self,
        *,
        openapi: str = VALID_OPENAPI,
        owner_map: str = VALID_OWNER_MAP,
        web_package: dict = VALID_WEB_PACKAGE,
    ) -> list[str]:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            openapi_path = root / "openapi.yaml"
            owner_map_path = root / "active-api-owner-map.md"
            web_package_path = root / "package.json"

            openapi_path.write_text(openapi, encoding="utf-8")
            owner_map_path.write_text(owner_map, encoding="utf-8")
            web_package_path.write_text(json.dumps(web_package), encoding="utf-8")

            return verify_contract(
                openapi_path=openapi_path,
                owner_map_path=owner_map_path,
                web_package_path=web_package_path,
            )

    def assertIssueContains(self, issues: list[str], expected: str) -> None:
        self.assertTrue(
            any(expected in issue for issue in issues),
            f"Expected issue containing {expected!r}, got: {issues!r}",
        )


if __name__ == "__main__":
    unittest.main()
