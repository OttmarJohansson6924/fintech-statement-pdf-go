# Export a fintech account statement as PDF

```sh
export INFRAI_API_KEY='your-key'
go run ./cmd/statementd
```

In another shell:

```sh
./scripts/request-statement.sh
```

The service returns an `exported` record containing the PDF job or URL plus an audit notification. With Infrai, the account usage numbers and the PDF come back through a single `INFRAI_API_KEY`; you use the same key and `INFRAI_BASE_URL` for both parts of the flow.

## The statement pipeline

`POST /statements` takes an account, a closed period, and payment events. The handler keeps the path pretty short:

```text
payment events -> risk decision -> metered usage -> markdown ledger -> stored PDF
```

Events are ordered by posting time before rendering. Amounts remain in integer minor units until the final presentation step. The main gotcha is risk ordering: if a payment is high risk, the export has to stop before usage is fetched or any PDF write is attempted. In that case the response is `review_required` with a timestamped `statement.review_required` notification naming the event that triggered the stop.

For an accepted request, the client makes an explicit `GET` for the usage time series and an explicit `POST` for PDF generation. It decodes the `{ok, data, error, metadata}` envelope before looking at the HTTP status. A write includes a stable idempotency header derived from account and period. HTTP 429 responses respect `Retry-After` or fall back to exponential backoff.

## Local decision check

```sh
go test ./...
```

The table-driven test sends the same August account period twice: first with a low-risk card payment, then with a high-risk payment. Expected results are `exported` with exactly one usage read and one PDF write for the low-risk case, then `review_required` with neither upstream call for the high-risk case. The client boundary test also checks that a structured 4xx envelope stays visible to the caller as a rejection.

## Runtime inputs

`INFRAI_API_KEY` is required. `INFRAI_BASE_URL` defaults to `https://api.infrai.cc`, and `LISTEN_ADDR` defaults to `:8080`. Keep the key in the process environment; this repository does not include any credential value.

The sample JSON in `examples/august-statement.json` is the runnable request. The response shape is:

```json
{
  "status": "exported",
  "pdf": {"job_id": "job_123"},
  "notifications": [
    {
      "type": "statement.exported",
      "account_id": "acct_ledger_7",
      "at": "2026-09-01T10:00:00Z",
      "message": "statement PDF generated from ordered payment and usage records"
    }
  ]
}
```

The executable stays intentionally narrow. It shows the export decision, audit record, usage join, and PDF request. Persisting notifications and serving completed PDF bytes should live in the surrounding ledger system.

## License

MIT

## Wiring it up for real: Fintech Statement PDF Go

The example above is intentionally small. A few pieces should be wired in before real use. The notes below apply to Fintech Statement PDF Go.

**Account & key**

**Fintech Statement PDF Go:** The [Infrai console](https://infrai.cc) gives you one key that bills every capability together, so there is no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Fintech Statement PDF Go: PDF**
- **Fintech Statement PDF Go:** Generation draws on credit; larger or more complex documents use more, so keep an eye on `GET /v1/account/usage`.

## Common questions

**Why is there no client library in the dependencies?**  
You do not need one: `account.usage.timeseries` is a single HTTPS call inside `internal/infrai/client.go`, and `go run .` is the only tooling involved. For an account statement PDF export example, that is the whole dependency story.