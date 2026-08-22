# Standing up an M365 Trial Tenant for mCollaborator SharePoint Testing

**Purpose.** Create a Microsoft 365 tenant where *you* are Global Administrator, so the
SharePoint integration can be built and tested end to end without any dependency on the
Cyberteq corporate tenant or its IT approval process.

**Outcome.** A working SharePoint site, an Entra ID app registration scoped to that single
site, and four config values the Go backend will consume.

> **Scope note.** This document gets the *tenant* ready. The SharePoint client code in the
> backend is separate work and does not exist yet — the runbook ends at a verified token +
> API call, which is the correct handoff point.

---

## 0. Read this first — the one mistake that breaks everything

When you sign up, **do not use your `@cyberteq.com` address as the account you sign in
with.** Microsoft resolves the domain of the sign-up identity: because `cyberteq.com` is
already claimed by Cyberteq's existing Entra tenant, the flow will try to add you to *that*
tenant as an ordinary member, and you will not be an admin. That is the exact opposite of
the goal here.

Use a **personal email** (Gmail/Outlook.com) as the sign-up contact. The trial then creates
a brand-new tenant with its own `something.onmicrosoft.com` domain, and makes you Global
Administrator of it.

Two other things to know before you start:

- **A credit card is required** even for the free month. You are charged when the trial
  converts unless you cancel. Set a calendar reminder for **day 25** now, before you forget.
- **Never put real client findings in this tenant.** It is a disposable sandbox with a
  30-day life, outside Cyberteq's governance and retention controls. Synthetic data only.
  This is the whole reason the sandbox is safe to use without approval.

---

## 1. Create the trial tenant (~15 min)

1. Go to `https://www.microsoft.com/microsoft-365/business` and pick **Business Basic** or
   **Business Standard** → *Try free for one month*.
   - Basic is sufficient: it includes SharePoint Online, which is all we need.
   - Standard adds the desktop Office apps — only worth it if you also want to open the
     generated DOCX reports inside this tenant.
2. Enter a **personal** email address when asked (see §0).
3. Complete phone verification.
4. Choose your tenant domain: `cyberteq-lab.onmicrosoft.com` or similar. This is permanent
   for the tenant, so pick something you can live with.
5. Create your admin account: `admin@cyberteq-lab.onmicrosoft.com`. **Store the password in
   a password manager** — there is no IT helpdesk to recover this for you.
6. Enter payment details and confirm.

Typical trial allowance is 25 user seats, which is ample for testing with a handful of
colleagues.

### Verify you are actually the admin

Sign in at `https://admin.microsoft.com`. If you can see the full admin centre navigation
(Users, Teams & groups, Settings, Billing), you have Global Administrator. If you instead
land on a stripped-down page, you have joined an existing tenant — go back and redo §1 with
a different sign-up email.

---

## 2. Create test users for your colleagues (~10 min)

Admin centre → **Users → Active users → Add a user**. For each colleague:

- Username: `analyst1@cyberteq-lab.onmicrosoft.com`
- Assign a licence (the trial licence) — without one they cannot access SharePoint
- Let it generate a password and choose *Require this user to change their password*

Send each person their sandbox credentials **out of band**. Note these are throwaway
sandbox identities and have no relationship to their real Cyberteq accounts.

> MFA is on by default in new tenants via security defaults. Leave it on — it costs you
> nothing here and mirrors what corporate will require later.

---

## 3. Create the SharePoint site (~5 min)

1. Admin centre → **Show all → SharePoint** (or `https://cyberteq-lab-admin.sharepoint.com`).
2. **Active sites → Create → Team site**.
3. Name: `VAPT` → this gives the URL
   `https://cyberteq-lab.sharepoint.com/sites/VAPT`.
4. Record that URL. You need it in §5.

---

## 4. Register the application (~15 min)

Go to `https://entra.microsoft.com` → **Applications → App registrations → New registration**.

| Field | Value |
|---|---|
| Name | `mCollaborator-Backend` |
| Supported account types | *Accounts in this organizational directory only* (single tenant) |
| Redirect URI | leave blank — this is a daemon app, not interactive |

After creating it, from the **Overview** blade record:

- **Application (client) ID**
- **Directory (tenant) ID**

Then:

1. **Certificates & secrets → New client secret**. Set expiry to 6 months. **Copy the
   secret value immediately** — it is never shown again.
2. **API permissions → Add a permission → Microsoft Graph → Application permissions** →
   add **`Sites.Selected`**.
3. Click **Grant admin consent for \<tenant\>**. You can do this because you are the admin —
   this is the step that is impossible in the corporate tenant without a ticket.

### Why `Sites.Selected` and not `Sites.ReadWrite.All`

`Sites.ReadWrite.All` grants the app read/write over **every** site in the tenant.
`Sites.Selected` grants nothing until you explicitly authorise individual sites, which is
what §5 does. Use `Sites.Selected` — partly because it is correct least-privilege, and
partly because when you eventually ask Cyberteq IT for this, `Sites.Selected` is a request
they can plausibly approve and `Sites.ReadWrite.All` is one they should refuse.

---

## 5. Grant the app access to just that site (~10 min)

`Sites.Selected` starts with zero site access. Granting it requires an admin identity with
`Sites.FullControl.All`, so do this interactively in **Graph Explorer**
(`https://developer.microsoft.com/graph/graph-explorer`), signed in as your tenant admin.

**5a. Get the site ID**

```http
GET https://graph.microsoft.com/v1.0/sites/cyberteq-lab.sharepoint.com:/sites/VAPT
```

The response `id` looks like
`cyberteq-lab.sharepoint.com,8f1c...,4a2b...`. Record the whole string.

**5b. Grant the app write access to that site**

```http
POST https://graph.microsoft.com/v1.0/sites/{siteId}/permissions
Content-Type: application/json

{
  "roles": ["write"],
  "grantedToIdentities": [
    {
      "application": {
        "id": "{client-id-from-step-4}",
        "displayName": "mCollaborator-Backend"
      }
    }
  ]
}
```

Graph Explorer will prompt you to consent to `Sites.FullControl.All` for *itself* to make
this call. That consent is for Graph Explorer, not for your app — your app keeps only
`Sites.Selected`.

**5c. Confirm the grant**

```http
GET https://graph.microsoft.com/v1.0/sites/{siteId}/permissions
```

You should see one permission entry naming `mCollaborator-Backend` with role `write`.

---

## 6. Verify the app can authenticate and reach the site (~5 min)

This is the acceptance test for the whole runbook. Run it from PowerShell on your machine —
nothing to do with the app yet, just proving the credentials work.

```powershell
$tenantId = "<directory-tenant-id>"
$clientId = "<application-client-id>"
$secret   = "<client-secret-value>"
$siteId   = "<site-id-from-5a>"

# get an app-only token
$body = @{
  grant_type    = "client_credentials"
  client_id     = $clientId
  client_secret = $secret
  scope         = "https://graph.microsoft.com/.default"
}
$token = (Invoke-RestMethod -Method Post `
  -Uri "https://login.microsoftonline.com/$tenantId/oauth2/v2.0/token" `
  -Body $body).access_token

# prove the token can read the granted site
Invoke-RestMethod -Uri "https://graph.microsoft.com/v1.0/sites/$siteId" `
  -Headers @{ Authorization = "Bearer $token" } | Select-Object displayName, webUrl
```

**Interpreting the result:**

| Outcome | Meaning |
|---|---|
| Site name + URL returned | Everything works. Proceed. |
| `401 Unauthorized` | Token wrong — check tenant ID, client ID, secret (secret *value*, not secret ID). |
| `403 Forbidden` | Token is valid but the §5b site grant is missing or targeted the wrong site. This is the common failure. |

---

## 7. Create the lists (~15 min)

Reproducible via script, because the trial expires and you will want to recreate this — in a
new sandbox, or eventually in the corporate tenant. Run it after §6 passes.

```powershell
# assumes $token and $siteId from section 6

function New-VaptList($name, $columns) {
  $body = @{ displayName = $name; list = @{ template = "genericList" }; columns = $columns } |
          ConvertTo-Json -Depth 6
  Invoke-RestMethod -Method Post `
    -Uri "https://graph.microsoft.com/v1.0/sites/$siteId/lists" `
    -Headers @{ Authorization = "Bearer $token"; "Content-Type" = "application/json" } `
    -Body $body
}

$sev = @("critical","high","medium","low","info")
$sta = @("draft","open","in_progress","remediated","closed")

New-VaptList "Engagements" @(
  @{ name = "EngagementId"; text = @{} },
  @{ name = "ClientName";   text = @{} },
  @{ name = "Status";       choice = @{ choices = @("planning","in_progress","review","closed") } },
  @{ name = "Methodology";  text = @{} },
  @{ name = "StartDate";    dateTime = @{} },
  @{ name = "EndDate";      dateTime = @{} }
)

New-VaptList "Findings" @(
  @{ name = "FindingId";    text = @{} },
  @{ name = "EngagementId"; text = @{} },
  @{ name = "Severity";     choice = @{ choices = $sev } },
  @{ name = "Status";       choice = @{ choices = $sta } },
  @{ name = "CvssScore";    number = @{} },
  @{ name = "CvssVector";   text = @{} },
  @{ name = "Cve";          text = @{} },
  @{ name = "Description";  text = @{ allowMultipleLines = $true } },
  @{ name = "Impact";       text = @{ allowMultipleLines = $true } },
  @{ name = "Remediation";  text = @{ allowMultipleLines = $true } },
  @{ name = "PocRef";       text = @{} },
  @{ name = "AssignedTo";   text = @{} },
  @{ name = "ItemVersion";  number = @{} }
)

New-VaptList "Nodes" @(
  @{ name = "NodeId";       text = @{} },
  @{ name = "EngagementId"; text = @{} },
  @{ name = "Target";       text = @{} },
  @{ name = "NodeType";     text = @{} }
)
```

Then, in the site UI, create two **document libraries**: `Evidence` and `Reports`.

### Two schema decisions worth understanding

**`PocRef`, not the PoC content.** SharePoint's multi-line text columns cap at 63,999
characters, and the app currently embeds PoC screenshots as base64 data-URIs, which blows
past that with a single image. `PocRef` holds a pointer to a file in the `Evidence` library
instead. This refactor is required for SharePoint and is worth doing regardless — see
`COLLABORATION-DESIGN.md` §2.3.

**Index `EngagementId` after creating the lists.** List settings → Indexed columns. Without
it you hit the 5,000-item view threshold once findings accumulate across engagements.

---

## 8. Record the configuration

Four values the backend will need. Put them in the environment, never in the repo — the
`.gitignore` covers `.env` patterns but the safest habit is to keep secrets out of the
working tree entirely.

```
SHAREPOINT_TENANT_ID=<directory-tenant-id>
SHAREPOINT_CLIENT_ID=<application-client-id>
SHAREPOINT_CLIENT_SECRET=<client-secret-value>
SHAREPOINT_SITE_ID=<site-id>
```

Rotate the secret when it expires (§4 set it to 6 months, which outlives the trial).

---

## 9. Before the trial ends

**Day 25 — decide.** Either cancel (Billing → Your products → Cancel subscription) or accept
the charge. Cancelling leaves the tenant in a reduced state and data is eventually purged.

**Keep the setup reproducible.** The §7 script plus this document *is* your recovery path.
Anything you configure by hand in the portal and do not write down is lost when the trial
dies. If you add columns or change the schema, update the script in the same commit.

**Extract the value before it expires.** The point of the sandbox is a working demo. Take
screenshots, capture request/response traces, note what the latency actually felt like with
several people editing — that evidence is what makes the corporate-tenant conversation
short.

---

## 10. Moving to the Cyberteq tenant later

What transfers and what does not:

| Item | Transfers? |
|---|---|
| §7 list schema script | Yes — re-runs against any site |
| App registration | No — must be re-registered in the corporate tenant by an admin |
| `Sites.Selected` grant | No — corporate admin must grant it per-site |
| Test data | No, and it should not |
| Your Global Admin rights | **No.** In the corporate tenant you are a user again. |

That last row is the point of the whole exercise: everything you can prove in the sandbox
becomes an argument for the one thing you cannot do yourself. Turn up with a working
integration, a least-privilege `Sites.Selected` request scoped to a single site, and
measured latency numbers, rather than a proposal.

---

## Troubleshooting

| Symptom | Cause |
|---|---|
| Sign-up put me in the Cyberteq tenant | Used a `@cyberteq.com` address. Redo §1 with a personal email. |
| `Grant admin consent` button greyed out | Not Global Admin — verify per §1. |
| `403` on Graph calls, token issues fine | §5b grant missing or wrong site ID. Check with §5c. |
| `401 invalid_client` | Used the secret **ID** instead of the secret **value**. |
| Colleague cannot open the site | No licence assigned (§2), or not added to the site's members group. |
| Everything worked, now `401` | Client secret expired. Create a new one (§4) and update the env. |
