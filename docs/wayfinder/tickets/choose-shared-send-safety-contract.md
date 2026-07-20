# Choose the shared send safety contract

Labels: `wayfinder:grilling`
Status: closed
Assignee: /root
Blocked by: `Verify the shared mail and send Graph contract`, `Choose shared resource identity and ID provenance`, `Choose shared capability and account compatibility rules`

## Question

What exact review snapshot, sender representation, Exchange-right diagnostic,
version binding, audit identity, confirmation language, and uncertain-outcome
behavior must govern `Mail.Send.Shared` without weakening CR-0066's fail-closed
send boundary?

## Resolution

Shared send remains an existing-draft, human-elicited, single-attempt operation.
It accepts only a signed shared-mailbox `resource_ref`; raw Graph IDs and
model-supplied confirmation values cannot reach the send route.

### Canonical review snapshot

Immediately before elicitation, build one canonical snapshot containing:

- immutable signed-in account and shared-resource IDs;
- signed-in account label and UPN snapshots;
- shared alias, display, resource owner, resource kind, and owner mailbox view;
- canonical configured owner mailbox address used as `From`;
- nonempty Graph `changeKey` and `isDraft=true`;
- subject;
- separate normalized To, Cc, and Bcc recipient multisets, with at least one
  recipient overall;
- every attachment page, binding each attachment's ID, name, size, content
  type, and inline status; and
- the documented owner Sent Items default.

Recipient ordering inside one field is not significant, but movement between
To, Cc, and Bcc is significant. Attachment replacement remains significant
even when visible name and size are unchanged. A missing canonical owner
mailbox address, empty change key, non-draft state, empty recipient set, or
incomplete attachment enumeration makes shared send unavailable.

The confirmation does not display message body content. It must state this
plainly and direct the human to review the body in Outlook rather than implying
the body was covered by the elicitation.

### Sender and confirmation language

The elicitation displays the signed-in delegate, shared alias, resource owner,
canonical From mailbox, separate To/Cc/Bcc recipients, subject, attachment
metadata, owner Sent Items default, and the irreversible external-send effect.
It requires an explicitly checked confirmation field.

Sender language is exact and non-predictive: Exchange chooses Send As or Send
on Behalf according to configured rights; recipients may see only the owner or
the delegate on behalf of the owner. The MCP cannot promise which. The owner's
Sent Items is the documented default, not a guarantee that it is the exclusive
copy location under tenant policy.

### Accepted-confirmation sequence

An accepted confirmation is single-use. After acceptance, perform this exact
sequence:

1. Rebuild the account and resource target from immutable IDs.
2. Reauthorize the exact shared send capability and token compatibility.
3. Revalidate the signed resource reference against that target.
4. Refetch the draft and all attachment pages through the owner route.
5. Require the same resource/view and presentation identity, `isDraft=true`,
   the same nonempty change key, canonical From, subject, To/Cc/Bcc multisets,
   and attachment identity/metadata.
6. Recheck authority immediately before the send POST.
7. Mark one send attempt and issue exactly one owner-routed POST.

Any identity, alias/display, route, draft, From, subject, recipient, or
attachment difference consumes the confirmation and requires a fresh review.
There is no field-comparison fallback when change key is missing. The MCP must
document Graph's unavoidable GET-to-POST race and must not claim atomic
send-only-this-version enforcement.

### Diagnostics and outcomes

No speculative send-right probe is performed because Graph cannot enumerate
the relevant Exchange rights. Local capability, token-context, and known-scope
failures may be stated definitively. `ErrorSendAsDenied` directs recovery toward
Send As or Send on Behalf. Generic access failures advise checking
`Mail.ReadWrite.Shared`, Full Access or folder access, and tenant policy without
claiming which condition failed.

No application or SDK retry is permitted for any send status. The send-attempt
marker is audit evidence, not an idempotency guarantee:

- cancellation or timeout before dispatch means no send started;
- explicit authorization rejection means failed, not uncertain;
- explicit `202` means accepted for Exchange processing, not delivered;
- cancellation, timeout, connection loss, or server failure after dispatch
  means outcome uncertain.

An uncertain result instructs the human to inspect the owner's Drafts and Sent
Items before doing anything else. The MCP never creates a replacement draft or
automatically retries. Any later attempt requires a new review whose
confirmation explicitly warns about the previous uncertain attempt.

### Audit evidence

Record stable account/resource IDs; masked presentation identities;
recipient/attachment counts; keyed fingerprints for the resource reference,
change key, subject, recipient multisets, and attachment set; confirmation
outcome; one send-attempt marker; Graph request ID and error class when
available; and final accepted, denied, failed, canceled, or uncertain outcome.

Do not record raw addresses, subject, attachment names, body, tokens,
authorization data, or upload URLs unless an existing explicit masking policy
permits a presentation value. Audit data is evidence only and never authorizes
or deduplicates a send.
