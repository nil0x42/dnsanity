package config

const VERSION = "v1.3.1"

const HEADER = `
  ▗▄▄▄ ▗▖  ▗▖ ▗▄▄▖ ▗▄▖ ▗▖  ▗▖▗▄▄▄▖▗▄▄▄▖▗▖  ▗▖
  ▐▌  █▐▛▚▖▐▌▐▌   ▐▌ ▐▌▐▛▚▖▐▌  █    █   ▝▚▞▘
  ▐▌  █▐▌ ▝▜▌ ▝▀▚▖▐▛▀▜▌▐▌ ▝▜▌  █    █    ▐▌
  ▐▙▄▄▀▐▌  ▐▌▗▄▄▞▘▐▌ ▐▌▐▌  ▐▌▗▄█▄▖  █    ▐▌
`

const DEFAULT_TEMPLATE = `
# <FQDN>              <EXPECTED ANSWER>

# gambling (censored on many countries)
bet365.com            A=5.226.17*

# pr0n (family censored)
xnxx.com              A=185.88.181.53 A=185.88.181.54 A=185.88.181.55 A=185.88.181.56 A=185.88.181.57 A=185.88.181.58 A=185.88.181.59 A=185.88.181.60 || A=185.88.181.53 A=185.88.181.54 A=185.88.181.55 A=185.88.181.56 A=185.88.181.57 A=185.88.181.58 A=185.88.181.59 A=185.88.181.60 A=185.88.181.*

# censored by many countries (AU/UK/DK/FR/GB/DE/RU/...)
sci-hub.se            A=186.2.163.*

# torrenting (frequently censored)
sukebei.nyaa.si       A=198.251.89.38

# chinese (far away, slow servers might TIMEOUT)
ustc.edu.cn           A=202.38.64.246

# Record-less registered SLD: allow SERVFAIL/FORMERR (not found),
# NOERROR (empty), or TIMEOUT (resolver keeps searching).
invalid.com           SERVFAIL || NOERROR || TIMEOUT || FORMERR

# resolvers MUST transmit CNAME info:
test.nextos.com       CNAME=sec-5413.nextos.com.cdn.cloudflare.net. A=172.64.145.241 A=104.18.42.15

# non-existent FQDN:
dn05jq2u.fr           NXDOMAIN

# japanese (far away, slow servers might TIMEOUT)
ftp.kddilabs.jp       A=192.26.91.193

# corporate TLD (non-legacy TLD, filters-out very old resolvers)
dns.google            A=8.8.8.8 A=8.8.4.4

# PS: Beware of geo-located domains for reliable results !
`

/*************** OLD DEFAULT TEMPLATE ENTRIES (LEGACY) *****************

#################### DISABLED #####################
# # RELIABLE (barely change over time):
#   cr.yp.to              A=131.193.32.108 A=131.193.32.109
#   algolia.net           A=103.254.154.6 A=149.202.84.123 A=*
#   ftp.gnu.org           A=209.51.188.20
#   mirrors.xmission.com  A=198.60.22.13
#   xmission.com          A=198.60.22.4
#   lists.isc.org         A=149.20.*

# UNRELIABLE
    # sometimes down:
        # app-c0a801fb.nip.io A=192.168.1.251
        # www-78-46-204-247.sslip.io A=78.46.204.247
    # this only works on DNSSEC-compatible servers:
        # dnssec-failed.org SERVFAIL

**********************************************************************/
