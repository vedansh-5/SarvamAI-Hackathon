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
| Create a web app with Lambda | $20 | not yet; deploying the VANS server from the command line did not count it, so it likely needs the console tour, like Bedrock |

**Who can do what**

| Identity | Can do | Used for |
|---|---|---|
| Root user | Everything | Signing in to the console. It also has an access key; the safer setup is none |
| `vans-deploy` (IAM user) | Only `infra/aws/vans-deploy-only.json`: in Mumbai, functions, tables, settings under `/vans/` and mail; buckets named `vans-*`; creating roles named `vans-*` that may do nothing but write logs; handing a `vans-*` role to Lambda; reading logs | Deploying the VANS server from this Mac |
| `vans-server-runtime` (IAM role) | `infra/aws/vans-server-runtime.json` plus writing logs: read and write `vans-*` tables, read settings under `/vans/`, send mail from Mumbai, read and write files in `vans-*` buckets. Only Lambda can take it on (`infra/aws/lambda-trust.json`) | What the VANS server runs as |
| `visdom-dev-backup` (IAM user) | Belongs to another project | Not used by VANS |

If `vans-deploy`'s permissions change, `infra/aws/vans-deploy-only.json` changes
with them, and a new version of the `vans-deploy-only` policy is made from the
file with the root profile (`aws iam create-policy-version ... --set-as-default`;
AWS keeps five versions, so delete the oldest first once there are five). The
runtime role's own policy is changed with `aws iam put-role-policy`, by the root
profile only: `vans-deploy` can hand the role to Lambda but cannot widen it.

Checked on 2026-10-14 with `vans-deploy`: tables, settings, mail and buckets
allowed; a table or setting not named for VANS, mail outside Mumbai, servers
(EC2) and adding a policy to the runtime role all refused.

**Also in the account, not VANS:** a stopped EC2 instance `visdom-loadgen` in
Mumbai with an 8 GB disk (about $0.70 a month, from credits).

## Model providers

| Provider | Role | Model | Key kept in |
|---|---|---|---|
| Google Gemini | Writing and routing first, pictures only (D-232, D-233) | `gemini-flash-lite-latest`, then `models/gemini-3.1-flash-lite` | `.env` as `VANS_LLM_API_KEY` |
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
| Cloudflare Workers AI token | `.env` | the VANS server on this Mac |
| Every model key and provider setting the server uses | AWS Parameter Store, Mumbai, under `/vans/server/`, encrypted (`SecureString`) | the VANS server on Lambda, once when it starts |
| Tester invite codes | the `vans-server` table | the VANS server; `vans-testers` |
| Console key (`VANS_CONSOLE_KEY`) | `.env`, and Parameter Store under `/vans/server/` | the VANS server; Vedansh, to sign in to the console |
| Console gate (`VANS_CONSOLE_GATE`) | Parameter Store under `/vans/server/`, and the `vans-console` Worker's secret `CONSOLE_GATE` | the VANS server and the Worker; nobody types it |
| Wrangler sign-in | `~/.wrangler` on this Mac (signed in 2026-10-14 with Vedansh's approval) | `npx wrangler` in `infra/cloudflare/console` |
| A key typed into Setup | the macOS Keychain, service `dev.vedansh.vans` | the engine |
| "VANS Developer" signing certificate | the login keychain; a copy in `~/.vans-signing` (back it up) | `codesign`, through `infra/mac/release.sh` |
| Update signing key | `~/.tauri/vans-updater.key` (back it up) | `infra/mac/release.sh`; copies already out check updates against its public half |
| A tester's invite code | the tester's Keychain, service `dev.vedansh.vans`, account `invite-code` | the tester build's engine |
| `vans-deploy` access key | `~/.aws/credentials`, profile `vans-deploy` | the AWS command line |
| Root access key | `~/.aws/credentials`, profile `default` | the AWS command line |
| `LIST_TO` | Cloudflare → Workers → `vans` → Settings → Variables and Secrets | the site's form |

The keys reach Parameter Store from `.env`: each `VANS_*` setting except
`VANS_TESTERS`, and `CLOUDFLARE_AI_TOKEN`, under the same name. When a key in
`.env` changes, it is changed there too (`aws ssm put-parameter --overwrite`),
and the server picks it up when it next starts (redeploying forces that).

## Checking on things

```bash
# AWS credits left and which credit activities are done (root profile)
aws freetier get-account-plan-state --query accountPlanRemainingCredits
aws freetier list-account-activities --query 'activities[].[status,title]' --output text

# The deploy user works and is fenced in (the last two should be refused)
aws sts get-caller-identity --profile vans-deploy
aws dynamodb list-tables --profile vans-deploy
aws ec2 describe-instances --profile vans-deploy
aws ses get-send-quota --region us-east-1 --profile vans-deploy

# The site and its form
curl -sI https://summonvans.in | head -1
```

## The VANS server on AWS (live since 2026-10-14)

| Piece | Name | Notes |
|---|---|---|
| Function | `vans-server`, Lambda, Mumbai, arm64, 256 MB, 140 s limit | built from `cmd/vans-lambda`; runs as `vans-server-runtime` |
| Its address | `https://dcebecm6rhedpylmqas2q3hxyq0holhl.lambda-url.ap-south-1.on.aws/` | a Lambda function URL; anyone can reach it, only an invite code gets an answer |
| Table | `vans-server`, DynamoDB, Mumbai, 10 reads and 10 writes a second, deletion protection on | testers (`pk = tester`), counts per day (`pk = use#<day>`), feedback (`pk = feedback`), one row per request (`pk = act#<day>`, deleted after 30 days by the `gone` time-to-live), and for email sign-in (D-281): which email has which code (`pk = email`), a code waiting to be typed, kept as a hash (`pk = join`, gone after two days), and the day's sign-up counts (`pk = joins#<day>`, gone after three) |
| Console | `https://console.summonvans.in` | sign in with `VANS_CONSOLE_KEY`, kept in `.env` and in Parameter Store (D-236). Served through the Cloudflare Worker `vans-console` (`infra/cloudflare/console`), which adds the `VANS_CONSOLE_GATE` secret; the function's own address answers "not found" for the console without it |
| Settings | Parameter Store, `/vans/server/*` | read into the environment when the function starts |
| Mail | SES, Mumbai, sandbox (200 a day, only to verified addresses) | sends feedback from `feedback@summonvans.in` to Vedansh's Gmail; sign-in codes from `VANS_JOIN_FROM` once that is set and production access is granted (D-281) |
| Logs | `/aws/lambda/vans-server`, kept 14 days | the tester's name and the kind of question; never what was asked (that is in the table, for the console) |

Releases for testers sit in the private bucket `vans-releases` (Mumbai,
public access blocked): `latest.json`, and per version the signed update
archive and the `.dmg`. The server reads it through `VANS_RELEASES_BUCKET` in
Parameter Store and hands out hour-long download links to testers only
(D-243). `infra/aws/releases-bucket.sh` made it.

```bash
./infra/mac/signing-setup.sh            # once: the signing certificate and the update key
./infra/mac/bump.sh 0.2.0               # set the version in every file that holds it
./infra/mac/release.sh "what changed"   # build, sign, check, upload and tag the version in tauri.conf.json
./infra/aws/deploy-server.sh            # build and deploy the server, label it with the commit, check it answers
go run ./cmd/vans-testers add Riya      # a new tester and their code (200 a day unless -daily); people who sign in by email get 100
go run ./cmd/vans-testers list          # every tester and what each used today
go run ./cmd/vans-testers off Riya      # switch a code off at once; "on" puts it back
go run ./cmd/vans-testers feedback      # what testers sent, newest first
```

**Versions** follow semantic versioning, `MAJOR.MINOR.PATCH`, and stay below
1.0 while VANS is in testing:

- **Minor** (0.1.2 to 0.2.0) when a release adds something a tester can see or
  do.
- **Patch** (0.2.0 to 0.2.1) when it only fixes what was there.
- **1.0.0** when VANS leaves testing.

The version lives in `tauri.conf.json`, `Cargo.toml`, `package.json` and its
lock file; `bump.sh` sets all of them and `release.sh` refuses to build if they
differ. A release is built only from a commit on `main`, and is tagged
`vX.Y.Z` on it, so `git log v0.2.0..v0.2.1` is what changed between two
releases. Earlier releases were tagged afterwards, matched to `main` by upload
time: v0.1.0 at #210, v0.1.1 at #215, v0.1.2 at #217. The server has no
version of its own: each deploy writes its commit into the function's
description, which `aws lambda get-function-configuration --function-name
vans-server --query Description` reads back.

Measured 2026-10-14 from this Mac: a health check in 0.12 to 0.17 seconds,
most of it opening the connection; "which application" answered in 0.65 to
1.0 seconds with the model; the function starts cold in about half a second
and uses 41 MB.

**Mail needed two things, both done on 2026-10-14** (the feedback is kept in
the table either way):

1. Click the link in the "Amazon Web Services – Email Address Verification
   Request" mail sent to the Gmail address.
2. In Cloudflare → summonvans.in → DNS, add three CNAME records, proxy off
   (DNS only):

   | Name | Target |
   |---|---|
   | `4igl4kajkflafkc4yssc6dtt4xkmucg3._domainkey` | `4igl4kajkflafkc4yssc6dtt4xkmucg3.dkim.amazonses.com` |
   | `mra6pnyjyc7wyay2bpiotezm4cqikkaf._domainkey` | `mra6pnyjyc7wyay2bpiotezm4cqikkaf.dkim.amazonses.com` |
   | `5fcyche54qiasw4tlzyzzuu52wbnzfvh._domainkey` | `5fcyche54qiasw4tlzyzzuu52wbnzfvh.dkim.amazonses.com` |

Check with `aws sesv2 get-email-identity --profile vans-deploy --email-identity summonvans.in --query VerifiedForSendingStatus`.

## Planned

| Piece | Where | Free allowance (check before relying on it) |
|---|---|---|
| App downloads and the update feed | S3, Mumbai | data out to the internet is free up to about 100 GB a month across the account |
| Dashboard of failures over time | read from the database, behind a login only Vedansh has | |
