# ADR-002: Domain Data APIs, not generic database APIs
Status: Accepted

Product applications never issue arbitrary SQL/table CRUD through a generic endpoint. They consume stable domain/query contracts. This preserves authorization, auditability, schema evolution and freedom to move data between PostgreSQL, cache, search, object storage or future databases.
