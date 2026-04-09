---
title: "Proposal: Toolbox-Level Image Embedding Pipeline"
type: docs
---

## Summary
Add **first‑class image ingestion + embedding** to MCP Toolbox without bloating individual connectors. Implement a **Toolbox‑level embedding pipeline** with pluggable model backends and storage adapters, then expose consistent tools (`embed_image`, `search_image`, `search_multimodal`) across supported databases.

This preserves the Toolbox’s core value proposition (unified MCP tools for data) while unlocking image‑RAG and multimodal workflows.

## Goals
- Provide a **single, consistent** image → vector pipeline.
- Keep the feature **Toolbox‑level**, not per‑connector hacks.
- Support **pluggable embedding backends** (Vertex AI, OpenAI, HF/CLIP, local).
- Support **incremental storage adapters** (BigQuery, Postgres/pgvector first).
- Minimize onboarding friction with an MVP quickstart.

## Non‑Goals
- No “native” image vectorization inside every database connector.
- No requirement to adopt external vector stores in v1.
- No new UI—CLI + MCP tools only.

## Architecture (High‑Level)
**New module:** `embedding` (Toolbox core)

```
Image Source -> Ingestion -> Embedder -> Vector -> Storage Adapter -> Search Tools
```

**Components**
- **Ingestion**: local path, HTTP URL, GCS/S3 object.
- **Embedder**: pluggable backend interface.
- **Storage adapter**: per‑database vector storage + query.
- **Tools**: `embed_image`, `search_image`, `search_multimodal`.

## Core Interfaces (Conceptual)
- `Embedder.EmbedImage(ctx, input) -> (vector, metadata, error)`
- `StorageAdapter.Store(ctx, vector, metadata) -> error`
- `StorageAdapter.Search(ctx, query_vector, filters, top_k) -> (results, error)`

## Tooling Surface
- **MCP Tools** (prebuilt):
  - `embed_image`
  - `search_image`
  - `search_multimodal` (text + image input)
- **CLI**:
  - `toolbox embed-images --config <file>` (batch ingestion)

## Config (Draft)
```yaml
embeddings:
  backend: gemini|vertexai|openai|hf
  model: <model-name>
  batch_size: 64
  image_sources:
    - type: gcs|local|http
      path: gs://bucket/path
storage:
  target: bigquery|postgres
  index_table: <name>
```

## Rollout Plan
**Phase 1 (MVP)**
- Embedding module + interface
- Backends: Vertex AI + HF/CLIP (local)
- Storage: BigQuery + Postgres/pgvector
- Docs + quickstart demo

**Phase 2**
- Elastic/OpenSearch, MongoDB vector adapters
- OpenAI backend

**Phase 3 (optional)**
- External vector stores (Pinecone/Weaviate)

## Why This Fits MCP Toolbox
- **Consistent tool surface** across data sources.
- Avoids connector bloat and inconsistent behavior.
- Easier to test in the growing MCP test harness.
- Aligns with the “prebuilt tools + custom tools” model.

## Risks / Mitigations
- **Model cost/latency** → batch mode + backend choice in config.
- **Storage heterogeneity** → adapter interface + phased rollout.
- **Security** → least‑privilege templates and explicit data‑flow docs.

## Success Metrics
- Time‑to‑first‑image‑search < 10 minutes.
- At least 2 production‑grade storage adapters.
- Integration examples for 2 MCP clients.

---

**Issue:** #2948
