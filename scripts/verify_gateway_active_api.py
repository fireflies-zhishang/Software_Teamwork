#!/usr/bin/env python3
"""Verify gateway active API contract metadata and owner-map drift."""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from dataclasses import dataclass
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError:  # pragma: no cover - exercised by CI failure output.
    yaml = None


HTTP_METHODS = {"get", "put", "post", "delete", "patch", "options", "head", "trace"}
ACTION_STYLE_SEGMENTS = {
    "login",
    "logout",
    "register",
    "download",
    "search",
    "generate",
    "export",
    "retry",
    "revoke",
}
OPERATIONAL_PATHS = {"/healthz", "/readyz"}
EXPECTED_FRONTEND_OPENAPI_SOURCE = "../../docs/services/gateway/api/openapi.yaml"


@dataclass(frozen=True)
class OperationRow:
    method: str
    path: str
    owner: str
    tag: str
    operation_id: str
    auth: str

    def as_tuple(self) -> tuple[str, str, str, str, str, str]:
        return (self.method, self.path, self.owner, self.tag, self.operation_id, self.auth)


def verify_contract(
    *,
    openapi_path: Path,
    owner_map_path: Path,
    web_package_path: Path,
) -> list[str]:
    issues: list[str] = []
    spec = load_openapi(openapi_path, issues)
    if not spec:
        return issues

    operations = collect_operations(spec)
    issues.extend(validate_operations(spec, operations))
    issues.extend(validate_missing_contracts(spec, operations))
    issues.extend(validate_frontend_generation(web_package_path))
    issues.extend(validate_owner_map(owner_map_path, spec, operations))
    return issues


def load_openapi(openapi_path: Path, issues: list[str]) -> dict[str, Any] | None:
    if yaml is None:
        issues.append("PyYAML is required; install it with `python -m pip install pyyaml`")
        return None
    try:
        content = openapi_path.read_text(encoding="utf-8")
    except OSError as exc:
        issues.append(f"Cannot read OpenAPI file {openapi_path}: {exc}")
        return None

    try:
        loaded = yaml.safe_load(content)
    except yaml.YAMLError as exc:
        issues.append(f"Cannot parse OpenAPI YAML {openapi_path}: {exc}")
        return None

    if not isinstance(loaded, dict):
        issues.append(f"OpenAPI file {openapi_path} must contain a YAML object")
        return None
    return loaded


def collect_operations(spec: dict[str, Any]) -> list[tuple[str, str, dict[str, Any]]]:
    paths = spec.get("paths")
    if not isinstance(paths, dict):
        return []

    operations: list[tuple[str, str, dict[str, Any]]] = []
    for path, path_item in paths.items():
        if not isinstance(path_item, dict):
            continue
        for method, operation in path_item.items():
            if method.lower() not in HTTP_METHODS or not isinstance(operation, dict):
                continue
            operations.append((method.upper(), str(path), operation))
    return operations


def validate_operations(
    spec: dict[str, Any],
    operations: list[tuple[str, str, dict[str, Any]]],
) -> list[str]:
    issues: list[str] = []
    root_security_defined = "security" in spec

    for method, path, operation in operations:
        label = f"{method} {path}"
        tags = operation.get("tags")
        responses = operation.get("responses")
        security_defined = "security" in operation or root_security_defined

        if not operation.get("operationId"):
            issues.append(f"{label} missing operationId")
        if not isinstance(tags, list) or not tags:
            issues.append(f"{label} missing tags")
        if not has_response_class(responses, "2"):
            issues.append(f"{label} missing 2XX response")

        if is_public_api_path(path):
            if not operation.get("x-owner-service"):
                issues.append(f"{label} missing x-owner-service")
            if not security_defined:
                issues.append(f"{label} missing security")
            if not has_response_class(responses, "4"):
                issues.append(f"{label} missing 4XX response")

        if is_stable_public_path(path):
            for segment in path_segments(path):
                if segment in ACTION_STYLE_SEGMENTS:
                    issues.append(f"{path} uses action-style segment `{segment}`")

    return issues


def has_response_class(responses: Any, prefix: str) -> bool:
    if not isinstance(responses, dict):
        return False
    return any(str(status_code).startswith(prefix) for status_code in responses)


def is_public_api_path(path: str) -> bool:
    return path.startswith("/api/v1/")


def is_stable_public_path(path: str) -> bool:
    return is_public_api_path(path)


def path_segments(path: str) -> list[str]:
    return [
        segment
        for segment in path.split("/")
        if segment and not (segment.startswith("{") and segment.endswith("}"))
    ]


def validate_missing_contracts(
    spec: dict[str, Any],
    operations: list[tuple[str, str, dict[str, Any]]],
) -> list[str]:
    issues: list[str] = []
    active = {(method, path) for method, path, _operation in operations}
    for placeholder in extract_missing_placeholder_operations(spec):
        parsed = parse_operation_label(placeholder)
        if parsed is None:
            issues.append(f"x-missing-contracts placeholder `{placeholder}` must be formatted as `METHOD /path`")
            continue
        if parsed in active:
            method, path = parsed
            issues.append(f"x-missing-contracts placeholder {method} {path} overlaps active paths")
    return issues


def extract_missing_placeholder_operations(spec: dict[str, Any]) -> list[str]:
    missing_contracts = spec.get("x-missing-contracts", [])
    if not isinstance(missing_contracts, list):
        return []

    placeholders: list[str] = []
    for entry in missing_contracts:
        if not isinstance(entry, dict):
            continue
        operations = entry.get("placeholderOperations", [])
        if isinstance(operations, list):
            placeholders.extend(str(operation) for operation in operations)
    return placeholders


def parse_operation_label(value: str) -> tuple[str, str] | None:
    parts = value.strip().split(maxsplit=1)
    if len(parts) != 2:
        return None
    method, path = parts
    return method.upper(), path


def validate_frontend_generation(web_package_path: Path) -> list[str]:
    try:
        package_json = json.loads(web_package_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return [f"Cannot read frontend package.json {web_package_path}: {exc}"]

    scripts = package_json.get("scripts", {})
    api_generate = scripts.get("api:generate") if isinstance(scripts, dict) else None
    if not isinstance(api_generate, str):
        return ["apps/web package.json missing scripts.api:generate"]
    if EXPECTED_FRONTEND_OPENAPI_SOURCE not in api_generate:
        return [
            "apps/web api:generate must use "
            f"{EXPECTED_FRONTEND_OPENAPI_SOURCE}, got `{api_generate}`"
        ]
    return []


def validate_owner_map(
    owner_map_path: Path,
    spec: dict[str, Any],
    operations: list[tuple[str, str, dict[str, Any]]],
) -> list[str]:
    try:
        content = owner_map_path.read_text(encoding="utf-8")
    except OSError as exc:
        return [f"Cannot read owner map {owner_map_path}: {exc}"]

    issues: list[str] = []
    expected_rows = expected_operation_rows(spec, operations)
    parsed = parse_owner_map(content)

    if parsed["active_count"] != len(expected_rows):
        issues.append(
            f"owner map active operation count differs from OpenAPI: "
            f"{parsed['active_count']} != {len(expected_rows)}"
        )

    expected_owner_summary = dict(Counter(row.owner for row in expected_rows))
    if parsed["owner_summary"] != expected_owner_summary:
        issues.append(
            "owner map owner summary differs from OpenAPI: "
            f"{parsed['owner_summary']} != {expected_owner_summary}"
        )

    expected_row_set = {row.as_tuple() for row in expected_rows}
    parsed_row_set = set(parsed["active_rows"])
    if parsed_row_set != expected_row_set:
        missing = sorted(expected_row_set - parsed_row_set)
        extra = sorted(parsed_row_set - expected_row_set)
        issues.append(
            "owner map active operation table differs from OpenAPI"
            f"; missing={missing[:5]}, extra={extra[:5]}"
        )

    expected_missing = set(extract_missing_placeholder_operations(spec))
    parsed_missing = set(parsed["missing_contracts"])
    if parsed_missing != expected_missing:
        issues.append(
            "owner map missing contracts differ from OpenAPI: "
            f"{sorted(parsed_missing)} != {sorted(expected_missing)}"
        )

    return issues


def expected_operation_rows(
    spec: dict[str, Any],
    operations: list[tuple[str, str, dict[str, Any]]],
) -> list[OperationRow]:
    root_security = spec.get("security") if "security" in spec else None
    rows: list[OperationRow] = []
    for method, path, operation in operations:
        tags = operation.get("tags") if isinstance(operation.get("tags"), list) else []
        security = operation["security"] if "security" in operation else root_security
        rows.append(
            OperationRow(
                method=method,
                path=path,
                owner=str(operation.get("x-owner-service") or owner_for_operational_path(path)),
                tag=str(tags[0]) if tags else "",
                operation_id=str(operation.get("operationId") or ""),
                auth=format_auth(security),
            )
        )
    return rows


def owner_for_operational_path(path: str) -> str:
    if path in OPERATIONAL_PATHS:
        return "gateway"
    return ""


def format_auth(security: Any) -> str:
    if security == []:
        return "none"
    if not isinstance(security, list) or not security:
        return "missing"

    names: set[str] = set()
    for requirement in security:
        if isinstance(requirement, dict):
            names.update(str(name) for name in requirement.keys())
    return ",".join(sorted(names)) if names else "none"


def parse_owner_map(content: str) -> dict[str, Any]:
    return {
        "active_count": parse_active_count(content),
        "owner_summary": parse_owner_summary(content),
        "missing_contracts": parse_missing_contract_table(content),
        "active_rows": parse_active_operations_table(content),
    }


def parse_active_count(content: str) -> int | None:
    match = re.search(r"Active operations:\s*`?(\d+)`?", content)
    return int(match.group(1)) if match else None


def parse_owner_summary(content: str) -> dict[str, int]:
    rows: dict[str, int] = {}
    for line in section_lines(content, "## Owner Summary"):
        cells = markdown_cells(line)
        if len(cells) < 2 or cells[0] in {"Owner service", "---"}:
            continue
        owner = strip_code(cells[0])
        count_text = strip_code(cells[1]).rstrip(":")
        if count_text.isdigit():
            rows[owner] = int(count_text)
    return rows


def parse_missing_contract_table(content: str) -> list[str]:
    rows: list[str] = []
    for line in section_lines(content, "## Missing Contracts"):
        cells = markdown_cells(line)
        if len(cells) < 1 or cells[0] in {"Placeholder operation", "---"}:
            continue
        operation = strip_code(cells[0])
        if parse_operation_label(operation) is not None:
            rows.append(operation)
    return rows


def parse_active_operations_table(content: str) -> list[tuple[str, str, str, str, str, str]]:
    rows: list[tuple[str, str, str, str, str, str]] = []
    for line in section_lines(content, "## Active Operations"):
        cells = markdown_cells(line)
        if len(cells) < 6 or cells[0] in {"Method", "---"}:
            continue
        rows.append(tuple(strip_code(cell) for cell in cells[:6]))
    return rows


def section_lines(content: str, heading: str) -> list[str]:
    lines = content.splitlines()
    in_section = False
    collected: list[str] = []
    for line in lines:
        if line.strip() == heading:
            in_section = True
            continue
        if in_section and line.startswith("## "):
            break
        if in_section:
            collected.append(line)
    return collected


def markdown_cells(line: str) -> list[str]:
    stripped = line.strip()
    if not stripped.startswith("|"):
        return []
    return [cell.strip() for cell in stripped.strip("|").split("|")]


def strip_code(value: str) -> str:
    value = value.strip()
    if value.startswith("`") and value.endswith("`") and len(value) >= 2:
        return value[1:-1]
    return value


def build_arg_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--openapi",
        type=Path,
        default=Path("docs/services/gateway/api/openapi.yaml"),
        help="Path to gateway OpenAPI YAML.",
    )
    parser.add_argument(
        "--owner-map",
        type=Path,
        default=Path("docs/services/gateway/docs/active-api-owner-map.md"),
        help="Path to the gateway active API owner map markdown file.",
    )
    parser.add_argument(
        "--web-package",
        type=Path,
        default=Path("apps/web/package.json"),
        help="Path to apps/web package.json.",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_arg_parser().parse_args(argv)
    issues = verify_contract(
        openapi_path=args.openapi,
        owner_map_path=args.owner_map,
        web_package_path=args.web_package,
    )
    if issues:
        print("Gateway active API contract verification failed:")
        for issue in issues:
            print(f"- {issue}")
        return 1
    print("Gateway active API contract verification passed.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
