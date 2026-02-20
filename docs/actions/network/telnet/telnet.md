# TELNET action

The TELNET action drives interactive Telnet sessions over plain TCP so FlowK can automate legacy network devices or embedded systems that still rely on line-based CLIs.  The implementation layers its socket handling on top of the [`github.com/reiver/go-telnet`](https://github.com/reiver/go-telnet) interfaces to remain compatible with existing Telnet tooling while still letting the action control timeouts and transcripts directly.

## Payload structure

```json
{
  "host": "10.0.0.5",
  "port": 23,
  "timeoutSeconds": 10,
  "readTimeoutSeconds": 5,
  "lineEnding": "CRLF",
  "steps": [
    { "connect": {} },
    { "expect": { "pattern": "login:" } },
    { "send": { "data": "admin" } },
    { "expect": { "pattern": "Password:" } },
    { "send": { "data": "1234", "mask": true } },
    { "expect": { "pattern": "(>|#) ", "timeoutSeconds": 5 } },
    { "send": { "data": "show version" } },
    { "expect": { "pattern": "Version\\s+([0-9.]+)", "capture": "version" } },
    { "close": {} }
  ]
}
```

* **host** (required): target hostname or IP address.
* **port** (optional): TCP port, defaults to `23`.
* **timeoutSeconds** (optional): connection and write deadline in seconds, defaults to `10`.
* **readTimeoutSeconds** (optional): default per-read timeout in seconds used by `expect` steps, defaults to `5`.
* **lineEnding** (optional): `CRLF`, `LF`, or `CR`. The default is `CRLF`.  This setting controls how `send` steps optionally append trailing characters and how the transcript normalises line endings.
* **steps** (required): ordered list of Telnet operations. Each step must contain exactly one of the following objects:
  * `connect`: open the TCP socket with the configured timeout.
  * `send`: transmit text to the peer.  Fields:
    * `data` (required): payload text.
    * `appendLineEnding` (optional, default `true`): whether to append the configured `lineEnding` after `data`.
    * `mask` (optional, default `false`): if `true`, the transcript records `****` instead of the actual text.
  * `expect`: read from the socket until the regular expression matches, or the timeout is hit.  Fields:
    * `pattern` (required): Go regular expression.
    * `timeoutSeconds` (optional): override for this step’s read timeout.  Defaults to `readTimeoutSeconds`.
    * `capture` (optional): when provided, stores the first capture group under that key in the result’s `captures` map.
  * `close`: close the socket.  If a workflow omits the explicit close step, the action closes the connection automatically once all steps finish.

## Behaviour

1. The first step must be `connect`.  Validation fails if the host/steps are missing, the port is outside the `1..65535` range, or a step declares multiple operations.
2. Each `send` operation honours the configured line ending and write timeout.  Masked sends log `****` to both the transcript and the FlowK logger to keep secrets out of logs.
3. `expect` steps stream incoming data into a rolling buffer until the expression matches.  Deadlines combine the step timeout (or default) with the task context deadline so cancellation propagates correctly.  When a match includes a capture group, the first group is stored in the result.
4. The transcript preserves the order of every interaction (including prompts, masked credentials, and echoed commands) so downstream actions can inspect the raw exchange.

## Result payload

The action returns JSON with `flow.ResultTypeJSON`:

```json
{
  "connected": true,
  "host": "10.0.0.5",
  "port": 23,
  "captures": {
    "version": "1.2.3"
  },
  "transcript": "login: admin\nPassword: ****\n> show version\nVersion 1.2.3\n"
}
```

* `connected`: indicates that the initial connection succeeded.
* `host` / `port`: echo the requested endpoint.
* `captures`: map of capture names to extracted values (empty when no captures run).
* `transcript`: concatenated view of everything written and read during the session.  Masked sends always appear as `****`.

## Logging

Every step emits descriptive log entries (connect, send, expect outcomes, and close) through the task logger so long-lived sessions remain observable without forcing the transcript to be parsed.

## Error handling

Failures (invalid payloads, timeouts, IO errors, unmatched patterns, or context cancellations) stop the action immediately and propagate a descriptive error message upstream.  The connection is closed in all failure scenarios before returning.
