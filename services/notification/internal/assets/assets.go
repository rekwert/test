package assets

import _ "embed"

// EmailLogo is the Cloud-hustle mark embedded for transactional mail (CID inline).
// Hosted /email-logo.png on the site is optional; CID does not depend on front deploy.
//
//go:embed email-logo.png
var EmailLogo []byte

const EmailLogoCID = "email-logo"
