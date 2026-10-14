# Where everything lives

The one place that says which account holds what, where every secret is kept,
what it costs and how to check on it. When something is added to an account,
moved, or a key is replaced, this file changes in the same pull request.

Secret values never go in this file or anywhere in the repository. This file
says where a secret is kept, never what it is.

Last checked: 2026-10-14.

## The split

| Home | Its job | What is there |
|---|---|---|
| **Cloudflare** | The public face | DNS for `summonvans.in`, mail forwarding, the website, the Windows form |
| **AWS, Mumbai (`ap-south-1`)** | What the app talks to | The VANS server, its database, the app's downloads and updates (all planned) |
| **Model providers** | Answering the model's questions | Gemini first, Groq when Gemini is overloaded |
| **GoDaddy** | Registration only | Holds `summonvans.in`; renews October 2027 |

Everything the app depends on at runtime sits in one account, in one region
near the people using it. Everything a visitor to the site touches sits in the
other. Why the line is drawn there is D-228.

## Cloudflare

| Thing | Name | Notes |
|---|---|---|
| Zone | `summonvans.in` | Free plan. Nameservers moved from GoDaddy on 2026-10-13 |
| Mail | `vedansh@summonvans.in` | Email Routing forwards it to Vedansh's personal inbox, filed under a `VANS` label. Receive only |
| Site | Workers project `vans` | Builds `website/` from `main`: root `/website`, `npm run build`, `npx wrangler deploy`. Merging to `main` publishes. Domains: `summonvans.in`, `www.summonvans.in`, `vans.vedanshsaini7719.workers.dev` |
| Form | `POST /api/windows` | Mails each entry from `windows-list@summonvans.in` through the `LIST_MAIL` binding. Stores nothing |
| Rules for the site | `website/RULES.md` | |

Cloudflare only sends to addresses verified under Email Routing → Destination
addresses. The forwarding address itself is not one, so `LIST_TO` holds the
verified personal address, and a mail filter on the sender files the entries
under the same label.

Workers Logs and Traces are off.

## AWS

| | |
|---|---|
| Account plan | Paid plan with sign-up credits. When the credits run out or expire, use is billed normally |
| Credits | $143.49 left on 2026-10-14 |
| Region | `ap-south-1` (Mumbai) for everything VANS runs |
| Budget | `My Zero-Spend Budget`: mails Vedansh as soon as anything costs more than $1 beyond the credits |

**Credits earned and still on offer** (Billing → Credits, or the command below):

| Activity | Credit | State |
|---|---|---|
| Launch an EC2 instance | $20 | earned |
| Set up a budget | $20 | earned |
| Create an Aurora or RDS database | $20 | earned; the database was deleted the same day |
| Use a model in the Bedrock playground | $20 | not yet; only the console tour counts, a call from the command line did not |
| Create a web app with Lambda | $20 | not yet; expected when the VANS server is deployed |

**Who can do what**

| Identity | Can do | Used for |
|---|---|---|
| Root user | Everything | Signing in to the console. It also has an access key; the safer setup is none |
| `vans-deploy` (IAM user) | Only `infra/aws/vans-deploy-only.json`: functions named `vans-*` in Mumbai, the one role they run as (allowed nothing but writing logs), and reading their logs | Deploying the VANS server from this Mac |
| `visdom-dev-backup` (IAM user) | Belongs to another project | Not used by VANS |

If `vans-deploy`'s permissions change, `infra/aws/vans-deploy-only.json` changes
with them, and the policy in the console is updated from the file. Its next
additions are the database table and the downloads bucket, both named `vans-*`.

**Also in the account, not VANS:** a stopped EC2 instance `visdom-loadgen` in
Mumbai with an 8 GB disk (about $0.70 a month, from credits).

## Model providers

| Provider | Role | Model | Key kept in |
|---|---|---|---|
| Google Gemini | Writing first, pictures only (D-232) | `models/gemini-3.1-flash-lite`, then `gemini-flash-lite-latest` | `.env` as `VANS_LLM_API_KEY` |
| Groq | Quick questions first (`openai/gpt-oss-20b`), writing second (`openai/gpt-oss-120b`) (D-232) | as named | `.env` as `VANS_LLM_BACKUP_API_KEY` |
| Cloudflare Workers AI | Last resort for writing (D-232) | `@cf/openai/gpt-oss-120b` | `.env` as `CLOUDFLARE_AI_TOKEN` (a token limited to Workers AI on this account) |

All are free plans that need no card, with limits per minute and per day that change without
notice. Measured 2026-10-14: Groq answered a small step request in about one
second, Workers AI in about two, and Gemini on the free plan takes one to
thirty-five seconds when busy. How the server chooses between them, and the
measured accuracy of each, is D-232; the lineup is `VANS_PROVIDERS` in `.env`.

## Where every secret is kept

| Secret | Kept in | Read by |
|---|---|---|
| Gemini key | `.env` at the repository root (never committed) | the engine on this Mac |
| Groq key | `.env` | the engine on this Mac |
| Cloudflare Workers AI token | `.env` | the VANS server |
| A key typed into Setup | the macOS Keychain, service `dev.vedansh.vans` | the engine |
| `vans-deploy` access key | `~/.aws/credentials`, profile `vans-deploy` | the AWS command line |
| Root access key | `~/.aws/credentials`, profile `default` | the AWS command line |
| `LIST_TO` | Cloudflare → Workers → `vans` → Settings → Variables and Secrets | the site's form |

Once the VANS server exists, the model keys move into it and the app carries
none.

## Checking on things

```bash
# AWS credits left and which credit activities are done (root profile)
aws freetier get-account-plan-state --query accountPlanRemainingCredits
aws freetier list-account-activities --query 'activities[].[status,title]' --output text

# The deploy user works and is fenced in (the second one should be refused)
aws sts get-caller-identity --profile vans-deploy
aws ec2 describe-instances --profile vans-deploy

# The site and its form
curl -sI https://summonvans.in | head -1
```

## Planned

| Piece | Where | Free allowance (check before relying on it) |
|---|---|---|
| VANS server: holds the model instructions and keys, checks invite codes, caps use per tester | Lambda, Mumbai | about 1 million requests a month |
| Database: invite codes, daily caps, failed-guide reports, usage counts | DynamoDB, Mumbai | 25 GB stored |
| App downloads and the update feed | S3, Mumbai | data out to the internet is free up to about 100 GB a month across the account |
| Dashboard of failures over time | read from the database, behind a login only Vedansh has | |
