# Registration Group Policy

`RegistrationGroupPolicy` is an optional JSON value stored in the existing
`options` table. It assigns an initial user group only when a user is first
created through password, WeChat, or OAuth registration.

```json
{
  "enabled": true,
  "default_group": "default",
  "source_overrides": {
    "password": "default",
    "wechat": "wechat-users",
    "oauth:github": "github-users",
    "oauth:company-sso": "company-users"
  }
}
```

Source keys are trimmed and normalized to lowercase. Built-in and custom OAuth
sources use the route provider name, so a custom provider uses
`oauth:<provider-slug>`. Two raw keys that normalize to the same source make the
stored policy invalid; the whole policy then safely falls back to `default`
instead of choosing a map entry nondeterministically.

A target is valid when it is `default`, a key in `GroupRatio`, or a top-level
key in `GroupGroupRatio`. An invalid source override falls back to the valid
policy default; a missing, disabled, unreadable, malformed, or otherwise invalid
policy falls back to `default` and never blocks registration. The policy is read
from the database for each new registration so changes do not depend on the
periodic option cache refresh.

Existing users, OAuth binding/login, and users created directly by an
administrator are not changed by this policy.
