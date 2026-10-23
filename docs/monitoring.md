# Monitoring and administration

[简体中文](monitoring.zh-CN.md)

## Public information

Settings independently controls anonymous access to system/hardware information,
expiration dates, monthly traffic/quotas and TCP probe history. System details
(including kernel and agent version) and expiration dates are hidden by default;
monthly traffic and probes remain enabled. These controls filter API responses,
not just the page. Admin sessions on the admin listener keep full access. Asset
fields, IP addresses, credentials and private notes are never public. Server
version is also hidden from anonymous metadata when system details are disabled.
Public notes and probe names are user-supplied public text: avoid secrets in them.
The public-page master switch still disables anonymous node data entirely.

## Network monitoring and alerts

Node cards show up to three configured targets when fresh TCP probe data is available. Nodes without
locally configured probes keep the same card layout. Node details display both TCP connect latency and the percentage of failed
connection attempts. This is not ICMP packet loss. Each agent supports up to 16
local targets; the homepage selection only changes display. Network history
defaults to 24 hours, using minute records for recent periods and hourly summaries for
longer periods (7 and 90 days of retention by default). The two periods are controlled
by `-keep-minute-days` and `-keep-hour-days` respectively. Probe targets are configured
only on each agent; the rule's optional target matches that local target name.
Leaving it blank monitors any reported target, without creating or modifying probes.

Network history aggregates each range into at most about 360 points. Charts use that
bucket size to identify gaps; demo data uses the same cadence and covers the full range.
Missing records break the curve instead of becoming zero values. All-failed buckets
retain their failure rate while latency remains unknown.

Homepage cards use compact columns with natural heights. Public descriptions move
into a hint beside the name, and tags sit at the bottom. Clicking a homepage card
or list row opens details in a dialog. Closing it, pressing Escape or using browser
Back preserves dashboard filters, sorting and scroll position. Direct detail links
and the Full page action still provide a standalone page. The network section shows
up to three targets' five-minute average latencies; hover to see each failure rate
and its calculation. Combined averages include only the displayed targets, weighted
by successful connections for latency and by total attempts for failures.
Each history block covers 30 seconds; hover or use arrow keys to inspect its values
and target coverage. Missing blocks stay unknown. These strips reuse the memory
window (at most 11 buckets per target), travel in the existing node response, obey
the public-probe visibility setting and add no database storage or requests.

Latency and failure-rate rules evaluate a five-minute in-memory probe window,
trigger at or above the configured threshold for the configured duration, and
recover below it. Latency averages successful attempts only; all-failed attempts
are unknown for latency and 100% for the failure rule. Data older than two minutes
is stale: it resets a pending duration and cannot generate a recovery. Keep the
local ping interval comfortably below two minutes (default 30 seconds) for these
alerts. Server restart resets the window; persisted firing state is retained.
Historical charts retain the existing minute aggregation and retention policy.

## Notification delivery

Alerts are queued durably for currently enabled channels. Two workers deliver
notifications, preserving order within each channel. Each delivery has at most
five attempts, with delays of 5 seconds, 30 seconds, 2 minutes and 10 minutes.
Pending work survives restart; an interrupted lease can be retried after one minute.
As with most retrying webhooks, a receiver can see a duplicate if delivery succeeds
but its acknowledgement is lost; this is not exactly-once delivery.

The queue holds at most 1,000 active deliveries. Capacity or persistence failure
is logged; it does not create unbounded workers or memory. The latest 100 records
and their attempts are shown in Alerts. Finished history is limited to 5,000
records and seven days. Disabling or deleting a channel cancels queued work;
already-dispatched requests may still reach the receiver. Retries use the current
channel configuration. Stored/displayed errors are fixed codes, never URLs,
credentials or remote response bodies. The existing Send test action remains a
direct test and is not part of alert delivery history.

## Availability

Node details provide 7-, 30- and 90-day views. A background worker observes each
node once per minute. Only adjacent equal known states, at most 90 seconds apart,
contribute known time. Uptime is online / (online + offline); coverage is known
time / the entire selected range. No known time produces no uptime percentage.

Startup, never-reported nodes, transitions, server downtime, delayed observation
and failed persistence leave explicit unknown gaps. The latest incomplete interval
is unknown too. Short outages between observations may be missed. This is an
estimate from the panel's perspective, not proof that a service was reachable.
Outage counts are observed offline runs; an unknown gap splits a run and does not
prove a recovery. Timeline results show the latest 100 intervals, while summary
totals cover the full range. Records are retained for 91 days and adjacent equal
intervals are merged. Node deletion removes its availability history.

## Assets and comparison

The node editor stores private provider, region, tags, price, currency, billing
cycle and an auto-renewal note. The note never triggers renewal or payment.
Costs normalize each recorded price by its billing months and group by currency,
without exchange-rate conversion. All priced nodes, including hidden, expired
and offline nodes, contribute. Zero-price/unpriced nodes do not contribute.
Monthly and annual amounts are rounded estimates, not invoices.

The public dashboard searches names, groups, public tags and public notes; inventory search
also covers private asset fields for administrators. Sort options handle missing
or hidden values. Select up to four visible nodes for timestamp-aligned live or
historical comparison. A shared URL identifies nodes and range, but grants no
additional permission and does not reveal hidden nodes.

## Node cards and public tags

Cards condense CPU core count and total memory into the system line, for example
`Debian 13 / amd64 / 2C4G / kvm`, without a separate configuration block or repeated disk capacity.
Online nodes show CPU usage, used/total memory and disk space, network rates and uptime.
Offline nodes hide live usage and retain the last reported configuration and monthly traffic;
unknown network rates are not presented as measured zero throughput. Totals are saved by the
existing background batch writer and survive server restarts. Totals missing from older data
are omitted from the shorthand until the next agent report. Cached configuration can be stale and follows the
system-information visibility setting; it neither implies that a node is online nor persists live usage.

In the node editor's Public display section, enter up to six public tags, each at most 20 characters,
separated by English/Chinese commas or newlines. They appear on cards, list rows and details and
participate in public search. Tags render only as text. They are separate from private asset tags,
which are never copied into public tags automatically. Public notes hold short descriptions;
keep sensitive information in private notes.

List rows retain the same configuration, public tags, notes, usage, traffic quota and expiration
information as cards. Alerts appear separately instead of replacing descriptions. Narrow screens
stack these groups to avoid squeezing them into desktop columns.
The CPU column also shows the latest 1/5/15-minute load averages below its usage meter for online nodes.

Hover over the traffic quota or traffic totals in either view to see the next reset date and time,
including the server's UTC offset at that instant. Set each node's reset day to 1–31; for purchase-day
billing, enter the day of the month on which it was purchased. Resets occur at midnight in the server's
timezone. If a month lacks that day, the reset uses its last day, then returns to the configured day
in subsequent months. This is a local monitoring estimate; match the provider's billing settings.
The reset timestamp is hidden along with traffic when public traffic is disabled.

## Sessions and audit

Security lists active sessions with a non-authenticating identifier, creation and
expiry times, IP and user agent. Revoke one session or all other sessions; revoking
the current session signs you out. Session tokens and hashes are never returned.

The audit records login attempts and authenticated configuration mutations,
including password/TOTP changes, node/rule/channel changes, token rotation and
session revocation. It stores only an allowlisted action, validated target ID,
time, HTTP status and IP, never request bodies or secrets. Records are written by
a background worker, retained for 30 days and capped at 10,000. Pages contain at
most 100 records with cursor pagination. The bounded queue holds 1,024 events plus
128 pending writes; overflow is counted and logged. A crash can lose unflushed
events; this local audit is not tamper-proof against a compromised server.

## Upgrade and verification

Upgrade the server first. New schema migrations 002–006 are additive; do not edit
the released 001 migration. Existing agents keep their local interval/configuration.
No agent dependency, inbound listener, disk write, command or update channel is added.
Back up the database before upgrading; older binaries cannot use new schema safely.

Run `make check`. On Windows, run `go test ./...`, `go vet ./...`, Linux-targeted
`go vet ./...` with `GOOS=linux CGO_ENABLED=0`, `pnpm --dir web test`,
`pnpm --dir web lint`, `pnpm --dir web typecheck` and `pnpm --dir web build`.
