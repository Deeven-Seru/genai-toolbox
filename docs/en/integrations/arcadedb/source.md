---
title: "ArcadeDB Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  ArcadeDB is a multi-model database with Bolt protocol support.
no_list: true
---

## About

[ArcadeDB][arcadedb-docs] is a multi-model database that supports graph (Cypher),
document (SQL), key-value, and time-series data in one engine. It exposes a
Bolt protocol endpoint compatible with the Neo4j driver.

[arcadedb-docs]: https://docs.arcadedb.com/

## Available Tools

{{< list-tools >}}

## Requirements

### Database User

This source uses standard authentication. Create an ArcadeDB user (or use the
`root` user) that can connect over Bolt.

## Example

```yaml
kind: source
name: my-arcadedb-source
type: arcadedb
uri: bolt://localhost:7687
user: root
password: ${PASSWORD}
database: "mydb"
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

## Reference

| **field** | **type** | **required** | **description**                                  |
|-----------|:--------:|:------------:|--------------------------------------------------|
| type      |  string  |     true     | Must be "arcadedb".                              |
| uri       |  string  |     true     | Bolt URI (e.g. "bolt://localhost:7687").         |
| user      |  string  |     true     | ArcadeDB user (e.g. "root").                     |
| password  |  string  |     true     | Password for the ArcadeDB user.                  |
| database  |  string  |     false    | Database name to connect to (default: "arcadedb"). |
