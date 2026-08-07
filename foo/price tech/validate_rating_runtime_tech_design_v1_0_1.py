#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path
import re
import sys
from collections import defaultdict, deque

import yaml
from jsonschema import Draft202012Validator

EXPECTED_STAGES = [
    "RequestValidation",
    "PurposeAndTimeResolution",
    "SnapshotValidation",
    "ProductEligibility",
    "ContractPolicyResolution",
    "VersionManifestResolution",
    "FactSelection",
    "UnitNormalization",
    "GeometryNormalization",
    "AddressAndGeographyClassification",
    "PackageFeatureEvaluation",
    "WeightCalculation",
    "Aggregation",
    "BaseRateLookup",
    "CandidateChargeGeneration",
    "ChargeBasisResolution",
    "ChargeMethodExecution",
    "ChargeComposition",
    "DerivedCharges",
    "BuySellTransformation",
    "MinimumAndCap",
    "MultiLegAndShipmentSummary",
    "CurrencyConversion",
    "RoundingFinalization",
    "EvaluationValidation",
    "RatingEvaluationBuild",
]

EXPECTED_ENUMS = {
    "CalculationPurpose": [
        "QUOTE", "ESTIMATED_COST", "ACTUAL_COST", "CUSTOMER_BILLING",
        "SIMULATION", "REPLAY", "DISPUTE_REVIEW"
    ],
    "PriceRole": ["CARRIER_TARIFF", "BUY", "INTERNAL", "SELL", "PUBLIC"],
    "ChargeEffect": ["ADD", "DEDUCT"],
    "EvaluationStatus": ["REQUESTED", "EVALUATING", "COMPLETED", "FAILED", "CANCELLED"],
    "RatingAggregationMode": [
        "PER_PACKAGE", "SHIPMENT_TOTAL", "FIRST_CONTINUE_SHIPMENT",
        "MASTER_PLUS_CHILD", "HYBRID"
    ],
}

REQUIRED_MODULES = {
    "shared-kernel", "decimal-kernel", "unit-kernel", "geometry-engine",
    "weight-engine", "rate-lookup-engine", "charge-engine", "currency-engine",
    "aggregation-engine", "compiled-plan-runtime", "stage-executor",
    "rating-application", "http-adapter", "postgres-adapter",
    "object-storage-adapter", "outbox-publisher"
}

FORBIDDEN_TERMS = [
    "EER",
]

def find_cycle(nodes, edges):
    graph = defaultdict(list)
    indegree = {n: 0 for n in nodes}
    for a, b in edges:
        graph[a].append(b)
        indegree[b] = indegree.get(b, 0) + 1
        indegree.setdefault(a, 0)
    q = deque(sorted([n for n, d in indegree.items() if d == 0]))
    seen = []
    while q:
        n = q.popleft()
        seen.append(n)
        for nxt in sorted(graph[n]):
            indegree[nxt] -= 1
            if indegree[nxt] == 0:
                q.append(nxt)
    return None if len(seen) == len(indegree) else [n for n, d in indegree.items() if d > 0]

def validate_manifest(manifest):
    errors = []
    stages = [x["name"] for x in sorted(manifest["execution_stages"], key=lambda x: x["sequence"])]
    if stages != EXPECTED_STAGES:
        errors.append("execution stages differ from normative semantics")

    if manifest.get("core_enums") != EXPECTED_ENUMS:
        errors.append("core enums differ from normative values")

    modules = {m["name"]: m for m in manifest["modules"]}
    missing = sorted(REQUIRED_MODULES - set(modules))
    if missing:
        errors.append(f"missing required modules: {missing}")

    edges = []
    for name, mod in modules.items():
        for dep in mod.get("depends_on", []):
            if dep not in modules:
                errors.append(f"module {name} depends on missing {dep}")
            else:
                edges.append((name, dep))
    cycle = find_cycle(set(modules), edges)
    if cycle:
        errors.append(f"module dependency cycle: {cycle}")

    forbidden = manifest.get("forbidden_dependencies", [])
    for rule in forbidden:
        from_layer = rule["from_layer"]
        to_layers = set(rule["to_layers"])
        for name, mod in modules.items():
            if mod["layer"] != from_layer:
                continue
            for dep in mod.get("depends_on", []):
                if modules[dep]["layer"] in to_layers:
                    errors.append(
                        f"forbidden dependency: {name}({from_layer}) -> "
                        f"{dep}({modules[dep]['layer']})"
                    )

    required_hot_path_forbidden = {
        "Redis", "ExternalMessageBroker", "WorkflowEngine",
        "RemoteRuleEvaluation", "NonDeterministicInference"
    }
    actual = set(manifest.get("semantic_hot_path_must_not_require", []))
    if not required_hot_path_forbidden.issubset(actual):
        errors.append("semantic hot-path forbidden dependency list is incomplete")

    return errors

def validate_openapi_mapping(manifest, openapi):
    errors = []
    operation_ids = {
        op["operationId"]
        for path_item in openapi.get("paths", {}).values()
        for method, op in path_item.items()
        if method in {"get", "post", "put", "patch", "delete"}
    }
    mapped = set(manifest.get("api_operation_mappings", {}))
    if operation_ids != mapped:
        errors.append(
            f"API operation mapping mismatch; missing={sorted(operation_ids-mapped)}, "
            f"extra={sorted(mapped-operation_ids)}"
        )
    return errors

def validate_plan(schema, example):
    errors = []
    validator = Draft202012Validator(schema)
    for error in sorted(validator.iter_errors(example), key=lambda e: list(e.path)):
        errors.append(f"plan schema {list(error.path)}: {error.message}")

    stage_names = [x["name"] for x in sorted(example["stage_plan"], key=lambda x: x["sequence"])]
    if stage_names != EXPECTED_STAGES:
        errors.append("plan example stage sequence mismatch")

    graph = example["charge_graph"]
    nodes = set(graph["nodes"])
    edges = [(x["from"], x["to"]) for x in graph["edges"]]
    for a, b in edges:
        if a not in nodes or b not in nodes:
            errors.append(f"charge edge references unknown node: {a}->{b}")
    cycle = find_cycle(nodes, edges)
    if cycle:
        errors.append(f"charge graph cycle: {cycle}")

    order = graph["topological_order"]
    if set(order) != nodes or len(order) != len(nodes):
        errors.append("charge topological_order does not contain each node exactly once")
    else:
        position = {n: i for i, n in enumerate(order)}
        for a, b in edges:
            if position[a] >= position[b]:
                errors.append(f"invalid topological order for {a}->{b}")
    return errors

def validate_markdown(path):
    text = Path(path).read_text(encoding="utf-8")
    errors = []
    if text.count("```") % 2:
        errors.append("markdown code fences are not balanced")

    headings = re.findall(r"^(#{1,6})\s+(.+)$", text, flags=re.M)
    normalized = []
    for hashes, title in headings:
        # Repeated generic appendix sub-headings are still checked by full level/title.
        normalized.append((len(hashes), title.strip()))
    duplicates = sorted({h for h in normalized if normalized.count(h) > 1})
    if duplicates:
        errors.append(f"duplicate headings: {duplicates[:10]}")

    for term in FORBIDDEN_TERMS:
        if re.search(rf"\b{re.escape(term)}\b", text):
            errors.append(f"forbidden project dependency term present: {term}")

    required_phrases = [
        "模块化单体",
        "Compiled Pricing Plan",
        "PostgreSQL",
        "Object Storage",
        "Redis 是可选",
        "26 阶段",
        "Golden Cases",
        "Replay",
    ]
    for phrase in required_phrases:
        if phrase not in text:
            errors.append(f"required design phrase missing: {phrase}")
    return errors

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("manifest")
    parser.add_argument("plan_schema")
    parser.add_argument("plan_example")
    parser.add_argument("openapi")
    parser.add_argument("design_md")
    args = parser.parse_args()

    manifest = yaml.safe_load(Path(args.manifest).read_text(encoding="utf-8"))
    schema = json.loads(Path(args.plan_schema).read_text(encoding="utf-8"))
    example = json.loads(Path(args.plan_example).read_text(encoding="utf-8"))
    openapi = yaml.safe_load(Path(args.openapi).read_text(encoding="utf-8"))

    errors = []
    errors += validate_manifest(manifest)
    errors += validate_openapi_mapping(manifest, openapi)
    errors += validate_plan(schema, example)
    errors += validate_markdown(args.design_md)

    result = {
        "status": "PASSED" if not errors else "FAILED",
        "manifest_version": manifest.get("manifest_version"),
        "module_count": len(manifest.get("modules", [])),
        "service_count": len(manifest.get("services", [])),
        "stage_count": len(manifest.get("execution_stages", [])),
        "api_operation_mappings": len(manifest.get("api_operation_mappings", {})),
        "plan_component_count": len(example.get("components", [])),
        "charge_graph_node_count": len(example.get("charge_graph", {}).get("nodes", [])),
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1

if __name__ == "__main__":
    raise SystemExit(main())
