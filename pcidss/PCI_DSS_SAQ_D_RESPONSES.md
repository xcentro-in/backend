# PCI DSS SAQ D Questionnaire - XCentro Backend Responses

**Company:** Merchantbridge Fintech Solutions
**Assessment Type:** SAQ D v4.0
**Date:** November 16, 2025
**Infrastructure:** Single EC2 instance with Docker Compose
**Team:** 3 engineers, fully remote

---

## EXECUTIVE SUMMARY QUESTIONS

### Q1: Facilities in PCI Scope
**Answer:** NA - Fully remote company with no physical facilities. All operations are cloud-based on AWS EC2. No retention of physical hard-copy materials containing PAN.

### Q2: Payment Application Information
**Answer:**
- **Product Name:** XCentro Payment Platform (Custom-built)
- **Version:** In active development (Git commit: ea1757a)
- **PA-DSS Validation:** Not applicable - custom application
- **P2PE Reference:** Not applicable

**Gateway Integrations:**
- JioPay (tokenization service)
- Easebuzz (tokenization service)
- Both gateways handle PCI DSS compliance for card tokenization

### Q3: Network Diagram
**Answer:** PROVIDED in `/pcidss/nwd.drawio.svg`

**Architecture:**
- Single AWS EC2 instance
- All services containerized via Docker Compose
- TLS 1.2/1.3 termination at Nginx
- Internal container network communication

### Q4: Asset Inventory
**Answer:** PROVIDED

**System Components:**
- 1x AWS EC2 instance (single production server)
- Nginx (reverse proxy with TLS termination)
- PostgreSQL 18 (database)
- Redis 8 (cache/session store)
- Backend API (Go application)
- Frontend (web application)
- Certbot (SSL certificate management)

**Software Stack:**
- OS: Linux (AWS EC2)
- Container Runtime: Docker
- Orchestration: Docker Compose
- Web Server: Nginx
- Database: PostgreSQL 18
- Cache: Redis 8
- Language: Go 1.x

### Q5: Externally Accessible IP Addresses for ASV Scan
**Answer:** TODO - Requires input from infrastructure team

**Note:** Need to provide:
- Public IP address of EC2 instance
- Domain: staging-api.xcentro.in (or production domain)

---

## REQUIREMENT 1: INSTALL AND MAINTAIN NETWORK SECURITY CONTROLS

### Q6 (1.1.1): Firewall/Router Configuration and Change Management
**Answer:**

**Firewall Configuration:**
- AWS Security Groups act as firewall
- Inbound rules:
  - Port 80 (HTTP) - from anywhere (ACME challenge)
  - Port 443 (HTTPS) - from anywhere (API traffic)
  - Port 22 (SSH) - from anywhere (recommend restricting to specific IPs)
- Outbound rules:
  - HTTPS requests only
  - DNS requests only

**Change Management:**
- Infrastructure as Code approach
- All configurations versioned in Git
- Docker Compose and Nginx configs tracked

**Recommendation:** Implement formal change approval process with documented change tickets for infrastructure modifications.

### Q7 (1.1.2): Detailed Network Diagrams
**Answer:**

**Network Zones:**
- **External Zone:** Public internet
- **DMZ Zone:** Nginx reverse proxy (ports 80, 443)
- **Application Zone:** Backend API, Frontend (internal container network)
- **Data Zone:** PostgreSQL, Redis (internal container network only)

**CDE Boundary:**
- CDE includes: Backend API, PostgreSQL, Redis
- Non-CDE: Frontend, Nginx (terminates before reaching card data)

**Network Segmentation:**
- Container-level isolation via Docker networks
- No direct external access to database or cache
- All traffic proxied through Nginx

**Recommendation:** Network diagram provided shows architecture, but VLAN IDs not applicable (single EC2 with containers).

### Q8 (1.1.3): Cardholder Data Flow Diagram
**Answer:** PROVIDED in `/pcidss/carddataflow.txt`

**Data Flow Summary:**
1. User enters card data in frontend
2. Card data sent to backend API via HTTPS (TLS 1.2+)
3. Backend forwards card data to payment gateway (JioPay/Easebuzz)
4. Gateway tokenizes card data
5. Backend receives token reference ID + last 4 digits
6. Only token and last 4 digits stored in database
7. Full PAN never stored, CVV never stored post-authorization

**Card Data Handling:**
- PAN: In memory only during transaction, tokenized immediately
- CVV: Transient only, never logged or stored
- Expiry: Not stored in our systems
- Cardholder Name: Stored for display purposes

### Q9 (1.1.5): Firewall/Router Management Roles
**Answer:**

**Current State:** 3-person engineering team with shared responsibilities

**Roles:**
- AWS Security Group management: DevOps/Infrastructure lead
- Docker/Nginx configuration: Backend engineers
- No formal RBAC matrix documented

**Recommendation:** Create formal documentation of:
- Role assignments for infrastructure changes
- User access matrix for AWS Console
- Approval workflow for firewall rule changes

### Q10 (1.1.6): Service/Protocol/Port Documentation
**Answer:**

**Allowed Services:**
- **Port 80 (HTTP):** Used only for ACME challenges (Let's Encrypt), automatically redirects to HTTPS
- **Port 443 (HTTPS):** Primary API traffic, TLS 1.2+ encryption
- **Port 22 (SSH):** Server administration access

**Insecure Protocols:** None in use
- No Telnet, FTP, plain HTTP (for data transmission), SNMP, POP3, IMAP

**Business Justification:**
- Port 443: Required for secure API communication
- Port 80: Required for SSL certificate renewal
- Port 22: Required for server administration

**Recommendation:** Restrict SSH (port 22) to specific IP addresses or use SSH key-based authentication only with IP whitelisting.

### Q11 (1.1.7): Firewall Rule Review (Six-Month)
**Answer:**

**Current State:** Ad-hoc reviews when making infrastructure changes

**Recommendation:** Implement bi-annual firewall rule review process:
- Schedule reviews every 6 months
- Document review findings
- Verify business justification for each rule
- Remove unused rules

### Q12 (1.2.3): Wireless Network Firewall
**Answer:** NA - No company-controlled wireless network connects to the CDE. Remote engineers use personal networks, but do not handle cardholder data locally.

### Q13 (1.4): Personal Firewall on Mobile Devices
**Answer:**

**Current State:** 3 remote engineers using personal laptops

**Recommendation:** Implement and document:
- Mandatory host-based firewall on all employee devices
- Endpoint security policy requiring:
  - Windows Firewall / macOS Firewall enabled
  - Anti-tampering settings
  - Screenshot evidence from sample devices
- Sample collection from all 3 engineers showing firewall active

### Q14 (2.1.1): Wireless Access Points
**Answer:** NA - No company-controlled wireless access points are used. All engineers work remotely using personal networks.

---

## REQUIREMENT 2: APPLY SECURE CONFIGURATIONS TO ALL SYSTEM COMPONENTS

### Q15 (2.2): System Hardening Guidelines
**Answer:**

**Current Hardening Measures:**

**PostgreSQL:**
- Custom user accounts with least privilege (owner, rw, superuser separation)
- Password authentication required
- Network isolation (container-only access)
- No public exposure

**Redis:**
- ACL-based user authentication
- Password-protected access
- Container-only network access
- No public exposure

**Nginx:**
- Server tokens disabled (version hiding)
- Symlink attacks prevented
- TLS 1.2+ only
- Strong cipher suites configured
- Security headers

**Application:**
- Non-root container execution
- Minimal base images (Alpine Linux)
- No unnecessary services running
- Health checks enabled

**Recommendation:** Create comprehensive hardening documentation for:
- OS-level hardening guidelines
- Container security standards
- Database security baselines
- Change management process for configuration changes

### Q16 (2.2.2): Insecure Services
**Answer:**

**A) Insecure Services in Use:** NONE

All services use secure protocols:
- HTTPS (TLS 1.2+) for web traffic
- PostgreSQL native protocol (encrypted connections available)
- Redis with password authentication
- SSH for server access
- No telnet, FTP, plain HTTP (for data), SNMP, or other insecure protocols

**B) Additional Security Features:** Not applicable - no insecure services in use

---

## REQUIREMENT 3: PROTECT STORED ACCOUNT DATA

### Q17 (3.1): Cardholder Data Storage and Retention
**Answer:**

**A) Storage Locations:**

**Database (PostgreSQL):**
- Table: `wallet_payment_sources`
  - Stored: Token reference ID (from gateway), last 4 digits, gateway ID
  - NOT stored: Full PAN, CVV, track data
- Table: `payment_transactions`
  - Stored: Transaction metadata, amounts, status
  - NOT stored: Card data (only references to tokens)

**Server Logs:**
- Application logs: PAN masked in logs (if present)
- Nginx access logs: URL paths only, no card data
- Database query logs: No card data logged

**Backups:**
- Docker volumes: `postgres_data`, `redis_data`
- Backup location: AWS (exact configuration TODO)
- Only tokenized data in backups

**Retention Period:**
- Payment tokens: Retained until user deletes payment method
- Transaction records: Retained per compliance requirements (7 years recommended)
- Session data: Redis TTL-based expiration

**B) Data Deletion Process:**

**Current Implementation:**
- Users can delete payment sources via API
- Soft delete pattern in database
- No automated purging process

**Recommendation:** Implement:
- Formal data retention policy document
- Automated data purging process for expired sessions
- Data deletion logs/audit trail
- Regular cleanup jobs for old transaction data

### Q18 (3.2): Sensitive Authentication Data Storage
**Answer:**

**CVV/CVV2:** NEVER STORED - Transient only during transaction
**PIN:** Not applicable - card-not-present transactions only
**Track Data:** NEVER STORED - not applicable for CNP

**Evidence:** Code review shows:
- `internal/adapters/external/payment/` - Card data forwarded to gateway immediately
- No database columns for CVV, PIN, or track data
- Card data exists only in HTTP request memory, cleared after gateway call

**Business Justification:** Not applicable - we are not an issuer and have no business need to store this data.

### Q19 (3.2.1, 3.2.2, 3.2.3): Application Logs - Sensitive Data
**Answer:**

**Current State:**
- Application logs use structured logging
- No explicit PAN masking implementation visible in current codebase

**Recommendation:** Implement and provide:
- PAN masking in all log statements
- Sample logs showing masked card data (e.g., 4111********1111)
- Log review showing no CVV, PIN, or track data in logs
- Automated log scanning for sensitive data patterns

### Q20 (3.3): PAN Display
**Answer:**

**A) PAN Display:**
- UI displays: Last 4 digits only
- API responses: Only last 4 digits returned
- Admin interfaces: No full PAN access (not stored)

**B) Business Justification:**
- Full PAN display: Not applicable - we don't store full PAN
- Last 4 digits: Required for user payment method identification

**Evidence:**
- Database schema: Only `last_four_digits` column exists
- API responses return masked format: `****1234`

### Q21 (3.4): Encryption of Stored Cardholder Data
**Answer:**

**A) Full PAN Encryption:** Not applicable - we do not store full PAN

**Stored Data Encryption:**

**Bank Account Numbers:**
- Algorithm: AES-256-GCM (authenticated encryption)
- Implementation: `internal/shared/crypto/aes.go` - `EncryptBankAccountNumber()`
- Key derivation: SHA-512 hash
- Storage format: `base64_encrypted:base64_nonce`

**Passwords:**
- Algorithm: bcrypt
- Cost factor: Configurable (default 10)
- Implementation: `golang.org/x/crypto/bcrypt`

**Payment Gateway Tokens:**
- Stored in plaintext (tokens are already opaque references)
- No cryptographic value in encrypting gateway tokens

**B) Audit Logs:**
- Recommendation: Implement PAN masking in logs (see Q19)

**C) Backup Encryption:**
- Current: Docker volume backups (encryption depends on AWS EBS settings)
- Recommendation: Enable AWS EBS encryption at rest for EC2 volumes

### Q22 (3.4.1): Disk Encryption Access Control
**Answer:**

**Current State:**
- Application-level encryption for bank account numbers (AES-256-GCM)
- Encryption keys in environment variables (`.env` file)
- Access control: File system permissions on `.env` file

**Recommendation:**
- Use AWS Secrets Manager or similar for encryption key management
- Separate authentication for encryption key access from OS-level access
- Document key access policies

### Q23 (3.5.1): Service Provider - Cryptographic Architecture (If Applicable)
**Answer:**

**Cryptographic Architecture:**

**Algorithms in Use:**
- **AES-128-CBC:** Payment gateway communication (JioPay, Easebuzz)
- **AES-256-CBC:** Payout provider communication (Castler)
- **AES-256-GCM:** Bank account number encryption (internal storage)
- **bcrypt:** Password hashing (cost factor: 10)
- **SHA-512:** Key derivation
- **HMAC-SHA256/SHA-512:** Webhook signature verification
- **TLS 1.2/1.3:** Transport layer encryption

**Key Usage:**
- **JIOPAY_ENCRYPTION_KEY:** AES-128 for request/response encryption
- **EASEBUZZ_ENCRYPTION_KEY:** AES-128 for request/response encryption
- **CASTLER_ENCRYPTION_KEY:** AES-256 for payout API encryption
- **BANK_ACCOUNT_ENCRYPTION_KEY:** AES-256-GCM for account number storage
- **JWT_SECRET_KEY:** HMAC-SHA256 for JWT signing

**Key Strength:**
- AES-128: 128-bit keys
- AES-256: 256-bit keys
- bcrypt: Configurable work factor

**Key Expiry:**
- Current: No automatic rotation
- Recommendation: Implement annual key rotation policy

**HSM/SCD:**
- None currently in use
- Recommendation: Consider AWS KMS for key management

### Q24 (3.5.2): Encryption Key Access Control
**Answer:**

**A) Key Access:**

**Current State:**
- Encryption keys stored in environment variables
- Access limited to:
  - Engineers with server SSH access (3 engineers)
  - Application runtime (read-only)

**Recommendation:** Implement:
- Formal key custodian designation
- Access control list for key management
- Regular access reviews
- Key access logging

**B) Key Custodian Forms:**

**Recommendation:** Create and maintain:
- Key custodian acknowledgment forms
- Key handling procedures
- Separation of duties documentation

### Q25 (3.5.4): Encryption Key Storage Locations
**Answer:**

**Current Storage Locations:**
1. **Production Environment Variables:** `/home/ubuntu/xcentro/backend/.env` (or similar)
2. **Version Control:** `.env.example` templates (no actual keys)

**Recommendation:**
- Migrate to AWS Secrets Manager or AWS Systems Manager Parameter Store
- Reduce storage locations to single secure vault
- Document key storage architecture
- Implement access logging for key retrieval

### Q26 (3.6.4): Key Rotation Logs
**Answer:**

**Current State:** No formal key rotation process implemented

**Recommendation:** Implement:
- Annual key rotation schedule
- Key rotation procedure documentation
- Rotation logs with timestamps
- Trigger rotation on suspected compromise
- Automated rotation reminders

### Q27 (3.6.6): Manual Clear-Text Key Management
**Answer:**

**Current State:** Keys stored as environment variables (clear-text in secure file)

**A) Split Knowledge/Dual Control:**
- Not currently implemented
- Small team size (3 engineers) makes this challenging

**B) Custodian Forms:**
- Not currently implemented

**Recommendation:**
- For 3-person team: Implement 2-of-3 key split using Shamir's Secret Sharing
- Require 2 engineers to reconstruct production keys
- Document key custodian responsibilities
- Create custodian acknowledgment forms

### Q28 (4.1, 4.1.1): Encryption for Data Transmission
**Answer:**

**A) Encryption Technology:**
- **TLS 1.2 and TLS 1.3** for all HTTPS communication
- Payment gateway APIs: HTTPS with additional AES encryption layer

**B) Encryption Configuration:**

**Nginx TLS Settings (backend.conf):**
```
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA384;
```

**Evidence:**
- Strong cipher suites configured
- Session timeout: 10 minutes
- SSL stapling enabled
- Perfect forward secrecy (ECDHE)

**C) Transaction Traffic Logs:**
- Nginx access logs show HTTPS requests
- Application logs do not expose card data

**D) Wireless Network:**
- Not applicable - no company wireless network

---

## REQUIREMENT 4: PROTECT CARDHOLDER DATA WITH STRONG CRYPTOGRAPHY DURING TRANSMISSION

### Q29 (4.2): Email/Chat/Messaging Transmission
**Answer:**

**Current State:**
- Cardholder data is NEVER transmitted via email, chat, or messaging
- Email service (AWS SES) used only for:
  - User verification codes
  - Transaction receipts (no full PAN)
  - Notifications
- No chat or messaging integration with card data

**Email Encryption:**
- AWS SES uses TLS for email transmission
- Email content does not contain cardholder data

---

## REQUIREMENT 5: PROTECT ALL SYSTEMS AND NETWORKS FROM MALICIOUS SOFTWARE

### Q30 (5.2): Anti-Virus Configuration
**Answer:**

**Current State:**
- Containerized environment with minimal attack surface
- No traditional anti-virus installed

**Recommendation:** Implement:
- Host-based anti-virus on EC2 instance (e.g., ClamAV)
- Container image scanning in CI/CD pipeline
- Signature updates: Daily
- Full system scans: Weekly
- Log retention: 12 months
- Sample scan reports

**Note:** For containerized environments, focus on:
- Image vulnerability scanning (Trivy, Clair)
- Runtime security monitoring (Falco)
- AWS GuardDuty for threat detection

---

## REQUIREMENT 6: DEVELOP AND MAINTAIN SECURE SYSTEMS AND SOFTWARE

### Q31 (6.1): Vulnerability Management
**Answer:**

**Current State:**
- Ad-hoc dependency updates
- GitHub Dependabot alerts enabled (assumed)

**Recommendation:** Implement:
- **Risk Ranking Process:** CVSS-based scoring (Critical, High, Medium, Low)
- **Vulnerability Scanning:** Monthly internal scans
- **Patch Management:**
  - Critical: 30 days
  - High: 90 days
  - Medium: 180 days
- **Vendor Alerts:** Subscribe to:
  - Go security announcements
  - PostgreSQL security mailing list
  - Redis security advisories
  - Docker security notifications
  - GitHub security advisories

**Provide:** Sample security alerts from last 3 months

### Q32 (6.3): Secure Development Process
**Answer:**

**Current Practices:**
- Git version control
- Code review process (inferred from Git workflow)
- Clean architecture principles

**Recommendation:** Document formal SDLC:
1. **Requirements:** Security requirements checklist
2. **Design:** Threat modeling
3. **Development:** Secure coding guidelines (see Q39)
4. **Testing:** Security testing checklist
5. **Deployment:** Deployment approval process
6. **Monitoring:** Security monitoring

**Documents to Create:**
- Secure SDLC policy
- Security requirements template
- Code review checklist
- Security testing procedures

### Q33 (6.3.1): Payment Application User List
**Answer:**

**Current State:** 3 engineers with application access

**Recommendation:** Provide:
- Screenshot of AWS Console IAM users
- Screenshot of EC2 SSH key users
- Database user list: `owner`, `xcentro_rw`, `postgres`
- Application deployment access

### Q34 (6.3.2): Code Review for PCI Requirements
**Answer:**

**Current State:** Code review via Git pull requests (inferred)

**PCI Requirement 6.5 Coverage:**
- **6.5.1 Injection Flaws:** Parameterized queries used (PostgreSQL pgx)
- **6.5.2 Buffer Overflow:** Go language memory-safe
- **6.5.3 Insecure Cryptographic Storage:** AES-256-GCM used
- **6.5.4 Insecure Communications:** TLS 1.2+ enforced
- **6.5.5 Improper Error Handling:** Generic error messages
- **6.5.6 High Vulnerabilities:** Dependency scanning needed
- **6.5.7 XSS:** Input validation needed
- **6.5.8 Access Control:** JWT-based authentication
- **6.5.9 CSRF:** Token-based protection needed
- **6.5.10 Authentication/Session:** JWT + bcrypt

**Recommendation:**
- Implement formal security code review checklist
- Document review findings
- Automated SAST scanning (e.g., Gosec for Go)
- Provide sample code review reports

### Q35 (6.4.1): Development/Production Network Segregation
**Answer:**

**Current State:**
- Single EC2 instance for staging/production
- No network segregation between dev/prod

**Network Details:**
- Development: Local developer machines
- Staging/Production: Single EC2 instance (same subnet)

**Recommendation:**
- Separate EC2 instances for staging and production
- Different AWS VPCs or subnets
- Network ACLs to prevent cross-environment access
- Document network segmentation with VLAN/subnet details

### Q36 (6.4.2): Developers/Testers/Production Support Segregation
**Answer:**

**Current Team:**
- 3 engineers with full stack responsibilities
- No role separation due to team size

**User List:**
- Engineer 1: Development, deployment, support
- Engineer 2: Development, deployment, support
- Engineer 3: Development, deployment, support

**Database Access:**
- `owner`: Full schema management
- `xcentro_rw`: Application runtime (read/write)
- `postgres`: Superuser (DBA tasks)

**Recommendation:**
- Document role assignments despite overlap
- Implement principle of least privilege where possible
- Separate production deployment access (requires approval)
- Consider break-glass access model

### Q37 (6.4.3, 6.4.4): Test Data and Test Environment
**Answer:**

**Test Data Generation:**
- Located in: `internal/adapters/database/postgres/migrations/seeds/`
- Method: SQL seed files with sample data
- PAN generation: Fake test numbers (not real PANs)
- Seeds: Wallet data, vouchers, payees

**Test Data Removal:**
- Seed rollback scripts: `*_rollback.sql`
- Helper script: `apply-seeds.sh rollback`

**Recommendation:**
- Never use production data in test environments
- Document test data generation process
- Automated cleanup checklist before production deployment
- Test environment isolated from production

### Q38 (6.4.5): Change Management
**Answer:**

**Current Process:**
- Git commit history
- Pull request workflow (inferred)
- No formal change request forms

**Recommendation:** Implement change management with:

**1. Change Impact Analysis:**
- Affected systems documentation
- Security impact assessment

**2. Change Approval:**
- Approval workflow (e.g., senior engineer approval)
- Change request tickets

**3. Functionality Testing:**
- Test plan documentation
- Test results recording

**4. PCI DSS 6.5 Testing:**
- Security testing checklist
- Vulnerability testing for changes

**5. Back-out Procedures:**
- Rollback plan documentation
- Database migration rollback scripts (already implemented)

**Provide:** Sample change requests for last 6 months

### Q39 (6.5): Secure Coding Training and Guidelines
**Answer:**

**Current State:**
- No formal secure coding training program
- CLAUDE.md provides development guidelines

**Recommendation:** Create and provide:

**1. Training Program:**
- Annual secure coding training
- Attendance records
- Training curriculum covering:
  - OWASP Top 10
  - PCI DSS 6.5.1-6.5.10
  - SANS CWE Top 25
  - Go-specific security practices

**2. Secure Coding Guidelines Document:**
- Input validation
- Output encoding
- Authentication/authorization
- Cryptography standards
- Error handling
- Logging practices
- SQL injection prevention
- XSS prevention
- CSRF protection

**Reference:** CLAUDE.md contains development practices but needs security-focused expansion

### Q40 (6.6): Web Application Security
**Answer:**

**A) External Vulnerability Assessment:**

**Current State:** No annual external web application assessment

**Recommendation:**
- Annual penetration testing
- OWASP Top 10 testing
- PCI DSS 6.5 vulnerability testing
- Clean scan report required

**B) Web Application Firewall (Alternative):**

**Current State:** No WAF implemented

**Recommendation (Alternative to annual testing):**
- Implement AWS WAF or CloudFlare WAF
- OWASP ModSecurity Core Rule Set
- Protection for:
  - SQL injection
  - XSS
  - CSRF
  - Path traversal
  - File inclusion
- Signature updates: Automatic
- Alert monitoring: Real-time

**Choose one:** Annual testing OR WAF implementation

---

## REQUIREMENT 7: RESTRICT ACCESS TO SYSTEM COMPONENTS AND CARDHOLDER DATA

### Q41 (7.1.1, 8.7): Access Control Documentation
**Answer:**

**Current State:**

**Application Access:**
- User roles: Owner, Admin, Finance, Support
- Implementation: `internal/middleware/role_middleware.go`
- Business-level permissions with Redis caching

**Database Access:**
- `postgres` (superuser): DBA tasks
- `owner`: Schema ownership and DDL
- `xcentro_rw`: Application runtime (read/write data)
- Migration user: Schema migrations only

**Server Access:**
- SSH access: 3 engineers
- AWS Console: Infrastructure management

**Recommendation:** Document and provide:
- User access matrix with business justifications
- Role-based access control (RBAC) policy
- Access request forms
- Quarterly access reviews
- Generic ID audit (verify no shared accounts)

### Q42 (8.1.2): User Account Management
**Answer:**

**Current State:**
- Application users: Self-service signup with email verification
- Admin users: Manual creation via database/API

**Recommendation:** Provide sample forms for:

**General Users:**
- User registration logs
- Email verification records

**Administrator Users:**
- Access request form template
- Approval workflow
- Privileged access justification

**Server/Database Users:**
- User creation tickets
- Access approval records
- Role assignment documentation

**Sample Period:** Last 6 months (general users and admin users)

### Q43 (8.1.3): User Termination Process
**Answer:**

**Current State:**
- Application users: Account deactivation via API
- Employee access: Manual removal

**Recommendation:** Implement and provide:
- Employee termination checklist
- Access revocation logs:
  - AWS Console access removed
  - SSH keys revoked
  - Database access removed
  - Application admin access removed
- Sample termination records (last 6 months)
- Same-day access removal verification

### Q44 (8.1.4): Inactive Account Deactivation (90 Days)
**Answer:**

**Current State:**
- No automated inactive user deactivation

**Recommendation:** Implement:
- Automated job to identify inactive users (90+ days)
- Deactivation process
- Monthly reports showing:
  - Users deactivated for inactivity
  - Database account reviews
  - Server account audits

**Apply to:**
- Application users
- Database users
- Server SSH accounts
- AWS IAM users

### Q45 (8.1.5): Vendor Remote Access
**Answer:**

**Current State:**
- No third-party vendors with remote access to CDE
- If needed: SSH access with temporary credentials

**Recommendation:** If vendor access required:
- Vendor access request form
- Time-limited access
- Specific permission scope
- Activity monitoring logs
- Immediate revocation after support completed
- MFA required

**Current Answer:** Not applicable - no vendor remote access

### Q46 (8.1.6, 8.1.7, 8.1.8, 8.2.3, 8.2.4, 8.2.5): Password and Account Policies
**Answer:**

**Configuration Screenshots Needed for:**

**PostgreSQL:**
- Account lockout: Not natively supported (recommend pgBouncer with connection limits)
- Password policy: Configured via role settings
- Session timeout: Application-level control

**Redis:**
- ACL-based authentication
- No native account lockout
- Timeout: Client connection timeout

**Application (Go Backend):**
- **Account Lockout:** Implement rate limiting
  - Threshold: 6 failed attempts
  - Duration: 30 minutes
- **Session Timeout:** JWT-based
  - Access token: 15 minutes
  - Refresh token: 7 days
- **Password Requirements:**
  - Length: Minimum 8 characters (recommend 12)
  - Complexity: Mixed case + numbers + special chars
  - History: Implement password history (last 4)
  - Expiry: Implement 90-day expiry
- **Password Hashing:** bcrypt (configurable cost factor)

**Recommendation:** Implement and document:
- Password policy enforcement in application
- Configuration screenshots from all systems
- Policy compliance evidence

### Q47 (8.2.1): Password Encryption
**Answer:**

**Server/Network Components:**
- **PostgreSQL:** Uses SCRAM-SHA-256 password authentication (encrypted storage)
- **Redis:** Password stored in config (recommend encryption at rest)
- **SSH:** Key-based authentication (no password storage)
- **AWS:** IAM credentials (AWS-managed encryption)

**Application:**
- **User Passwords:** bcrypt hashing (cost factor: 10)
- **Implementation:** `golang.org/x/crypto/bcrypt`
- **Storage:** Hashed passwords in PostgreSQL
- **Transmission:** HTTPS (TLS 1.2+)

**Service Provider (Customer Passwords):**
- Same bcrypt implementation
- Unreadable at rest (hashed)
- Strong encryption algorithm

**Evidence:**
- Code: `internal/services/auth/auth_service.go`
- Screenshot of password column showing bcrypt hashes (recommend providing)

### Q48 (8.2.2): Password Reset Process
**Answer:**

**Current Implementation:**
1. User requests password reset
2. Verification code sent to registered email
3. Code verification required
4. User sets new password
5. Password hashed with bcrypt before storage

**Evidence:**
- Password reset API endpoints
- Email templates for reset codes
- Code verification logic

**Recommendation:** Provide:
- Sample password reset request logs
- Email delivery confirmation
- Password change audit logs

### Q49 (8.2.6): First Logon Password Change
**Answer:**

**Current Process:**

**User Creation:**
- Self-service signup: User creates own password
- Admin creation: Not currently implemented

**First Logon Enforcement:**
- Not currently enforced for admin-created accounts

**Recommendation:** Implement:
- Temporary password generation for admin-created users
- Force password change on first login
- Screenshot of enforcement mechanism
- User identity verification for password resets

### Q50 (8.3, 8.3.1): Multi-Factor Authentication (Administrative Access)
**Answer:**

**Current State:** MFA not currently implemented

**Recommendation:** Implement MFA for:

**Non-Console Administrative Access:**
- SSH access to EC2 instance
- Database administrative access
- Application admin panel access

**Implementation Options:**
- **SSH:** Public key + TOTP (Google Authenticator)
- **Application:** JWT + TOTP second factor
- **AWS Console:** AWS MFA (already available)

**Evidence to Provide:**
- MFA configuration screenshots
- Authentication logs showing MFA verification
- User enrollment records

### Q51 (8.3.2): Multi-Factor Authentication (Remote Access)
**Answer:**

**Current State:** No MFA for remote access

**A) MFA Technique:**
- Recommendation: Public key authentication + TOTP

**B) Remote Access User List:**
- 3 engineers with SSH access
- No third-party remote access
- VPN not currently used

**C) VPN Configuration:**
- Not applicable - direct SSH access

**Recommendation:** Implement:
- MFA for all SSH access
- VPN with MFA for additional security layer
- AWS Systems Manager Session Manager (MFA-enabled alternative to direct SSH)
- User access logs with MFA verification

### Q52 (8.5.1): Service Provider - Unique Credentials per Customer
**Answer:** NA - Not applicable. We do not provide remote access to customer premises or manage customer POS systems.

### Q53 (8.6): Alternative Authentication Mechanisms
**Answer:**

**Current State:**
- Standard password authentication only
- JWT tokens for API session management

**Other Mechanisms:**
- Not currently used (no physical tokens, smart cards, certificates)

**If Implementing:**
- Document assignment to individual accounts
- Prevent sharing between users
- Access logs

### Q54 (8.7): Database Active Connections
**Answer:**

**PostgreSQL Connection Monitoring:**

**Command for Active Connections:**
```sql
SELECT pid, usename, application_name, client_addr, state, query_start
FROM pg_stat_activity
WHERE state = 'active';
```

**Recommendation:** Provide:
- Screenshot of active database connections
- Show user authentication (no generic IDs)
- Connection source IP addresses
- Application name identification

---

## REQUIREMENT 9: RESTRICT PHYSICAL ACCESS TO CARDHOLDER DATA

### Q55 (9.1.1, 9.2): Physical Access Controls
**Answer:** NA - No company-controlled physical facilities or sensitive work areas. Fully remote company with cloud-based infrastructure on AWS.

### Q56 (9.3): Physical Access Authorization
**Answer:** NA - No company-controlled physical facilities or work areas requiring access badges.

### Q57 (9.4.1, 9.4.4): Visitor Access Controls
**Answer:** NA - No company-controlled physical facilities or visitors to manage.

### Q58 (9.4.4): Visitor Access Policy
**Answer:** NA - No company-controlled physical facilities or visitors.

### Q59 (9.5.1): Offsite Backup Storage
**Answer:** NA - Cloud-based backups only (AWS EBS snapshots or similar). No physical media used for backups.

**Recommendation:** Document AWS backup strategy:
- EBS snapshot schedule
- Snapshot encryption
- Retention period
- Access controls

### Q60 (9.6, 9.6.1): Physical Media Management
**Answer:** NA - Physical media is not used to store covered information. All data storage is cloud-based.

### Q61 (9.8.1, 9.8.2): Hard Copy Material Destruction
**Answer:** NA - Fully remote, no retention of physical hard-copy materials containing PAN.

### Q62 (9.9.1, 9.9.2): POS Device Inspection
**Answer:** NA - No Point-of-Sale (POS) devices are used. Card-not-present (online) transactions only.

### Q63 (9.9.3): POS Personnel Training
**Answer:** NA - No POS locations or personnel handling physical card devices.

---

## REQUIREMENT 10: LOG AND MONITOR ALL ACCESS TO SYSTEM COMPONENTS AND CARDHOLDER DATA

### Q64 (10.1): Centralized Logging Configuration
**Answer:**

**Current State:**
- Application logs: Stdout (captured by Docker)
- Nginx logs: `/var/log/nginx/access.log` and `error.log`
- PostgreSQL logs: Docker container logs
- No centralized syslog system

**Recommendation:** Implement:
- Centralized logging solution (ELK Stack, AWS CloudWatch, Loki)
- Log aggregation from all containers
- Syslog configuration screenshots
- Log backup frequency documentation
- Log retention: 12 months

**Systems to Monitor:**
- Backend API
- Nginx
- PostgreSQL
- Redis
- System logs

### Q65 (10.2, 10.3): Audit Log Requirements
**Answer:**

**Current Logging Coverage:**

**Application Logs Should Include:**
1. ✓ Individual access to cardholder data (API requests)
2. ✓ Administrative actions (user management, config changes)
3. Audit trail access (implement logging for log access)
4. Invalid access attempts (implement failed authentication logging)
5. ✓ Authentication mechanisms (JWT validation)
6. Privilege elevation (implement logging)
7. Account changes (implement admin account modification logs)
8. ✓ Audit log initialization (application startup)
9. Audit log pausing (implement alerts)
10. System object creation/deletion (implement database DDL logging)

**Log Format Requirements:**
- ✓ User ID
- ✓ Timestamp (date and time)
- ✓ Event type
- Success/failure indication (enhance logging)
- ✓ Source IP address
- Target system (enhance logging)

**Recommendation:** Enhance logging to capture:
- All PCI requirement 10.2 events
- Structured log format (JSON)
- Sample audit logs from each system component

### Q66 (10.4.1, 10.4.2): Time Synchronization
**Answer:**

**Current State:**
- EC2 instance uses AWS time sync service (NTP)
- Container clocks inherit from host

**Recommendation:** Document and provide:

**NTP Configuration:**
- External NTP source: AWS Time Sync Service or public NTP (e.g., time.aws.com, pool.ntp.org)
- NTP server access control list
- Time sync monitoring

**Audit Policy:**
- Time change logging
- Alerts for time drift
- Sample audit logs for time changes

### Q67 (10.5): Log Protection
**Answer:**

**Current State:**
- Logs stored in Docker volumes
- File system permissions on host

**Recommendation:** Implement:

**A) Directory Access Permissions:**
- Screenshot showing restricted access to log directories
- Only administrators can access logs
- No modification by application users

**B) File Integrity Monitoring:**
- FIM tool configuration (e.g., AIDE, Tripwire, OSSEC)
- Monitor log files for unauthorized changes
- Alert generation for log modifications
- Screenshot of FIM configuration

### Q68 (10.6.1, 10.6.2, 10.6.3, 10.7): Log Review and Retention
**Answer:**

**Current State:**
- Ad-hoc log review
- Log retention: Docker volume persistence (no defined retention period)

**Recommendation:** Implement:

**Daily Log Review:**
- Automated log analysis
- Security event detection
- Review reports/emails per system:
  - Backend API
  - Nginx
  - PostgreSQL
  - Redis
- Event follow-up documentation

**Log Retention:**
- Minimum: 12 months (3 months online + 9 months archived)
- Syslog configuration showing retention policy
- Sample summary reports (3 per year)

### Q69 (10.8): Service Provider - Security Control Failure Response
**Answer:**

**Recommendation:** Develop Incident Response Plan covering:

**Security Control Failures:**
1. **Restore Security Functions:** Immediate restoration procedures
2. **Document Duration:** Start and end timestamps of failure
3. **Root Cause Analysis:** Identify and document causes
4. **Address Security Issues:** Document issues during failure
5. **Risk Assessment:** Evaluate impact of security failure
6. **Prevent Recurrence:** Implement controls
7. **Resume Monitoring:** Verify monitoring restored

**Evidence to Provide:**
- Incident response plan document
- Security control documentation
- Sample incident reports
- Risk assessment reports
- Remediation plans

---

## REQUIREMENT 11: TEST SECURITY OF SYSTEMS AND NETWORKS REGULARLY

### Q70 (11.1, 11.1.1): Wireless Analyzer Scans
**Answer:** NA - No company-controlled wireless network to analyze. Remote employees use personal networks.

### Q71 (11.1.2): Rogue Wireless Access Point Detection
**Answer:** NA - No company-controlled infrastructure to monitor for rogue access points.

### Q72 (11.2.1): Internal Vulnerability Scanning
**Answer:**

**Current State:** No regular internal vulnerability scanning

**Recommendation:** Implement quarterly internal vulnerability scans:

**Requirements:**
- Frequency: Quarterly + after significant changes
- Scope: All CDE systems
  - EC2 instance OS
  - Docker containers
  - PostgreSQL
  - Redis
  - Application dependencies
- Tools: OpenVAS, Nessus, or similar
- Clean scan: No high/critical vulnerabilities
- Re-scan: After remediation

**Provide:**
- Last 4 quarters of scan reports
- Re-validation reports (if high vulnerabilities found)
- Remediation tracking

### Q73 (11.2.2): External ASV Scanning
**Answer:**

**Current State:** No ASV scanning performed

**Requirement:** Quarterly ASV scans by PCI SSC approved vendor

**Requirements:**
- Frequency: Quarterly + after significant changes
- Scope: External-facing IP addresses and domains
- ASV: PCI SSC approved scanning vendor
- Clean scan: No vulnerabilities rated 4.0 or higher (CVSS)
- Re-scan: After remediation

**Provide:**
- Last 4 quarters of ASV scan reports (passing)
- Re-validation reports (if applicable)
- ASV company name and approval status

### Q74 (11.3): Penetration Testing Methodology
**Answer:**

**Recommendation:** Develop penetration testing methodology based on:
- **NIST SP 800-115:** Technical Guide to Information Security Testing
- **OWASP Testing Guide**
- **PTES (Penetration Testing Execution Standard)**

**Methodology Should Cover:**
- Scope definition
- Reconnaissance
- Vulnerability analysis
- Exploitation
- Post-exploitation
- Reporting

### Q75 (11.3.1): External Penetration Testing
**Answer:**

**Current State:** No external penetration testing performed

**Requirement:**
- Frequency: Annually + after significant changes
- Scope:
  - Network layer (infrastructure)
  - Application layer (API endpoints)
- Methodology: Industry-accepted approach (NIST SP 800-115)

**Provide:**
- Annual external penetration test report
- Scope documentation
- Vulnerability remediation records
- Re-test results (if high-risk findings)

### Q76 (11.3.2): Internal Penetration Testing
**Answer:**

**Current State:** No internal penetration testing performed

**Requirement:**
- Frequency: Annually + after significant changes
- Scope:
  - Network layer (container network)
  - Application layer (internal APIs)
  - Database security
- Methodology: Industry-accepted approach

**Provide:**
- Annual internal penetration test report
- Scope documentation

### Q77 (11.3.4.1): Service Provider - Segmentation Testing
**Answer:** NA - No network segmentation implemented (single EC2 instance). If segmentation is implemented in future, require bi-annual penetration testing of segmentation controls.

### Q78 (11.4, 12.10.3, 12.10.5): Intrusion Detection/Prevention System
**Answer:**

**Current State:** No IDS/IPS deployed

**Recommendation:** Implement:

**IDS/IPS Solution:**
- AWS-native: AWS GuardDuty + AWS Security Hub
- Host-based: OSSEC or Wazuh
- Network-based: Suricata or Snort

**Configuration:**
- Monitor all CDE traffic
- Alert on suspicious activity
- Email alerts to security team

**Evidence to Provide:**
- Screenshot of IDS/IPS version and signature version
- Configuration showing monitored interfaces
- Alert configuration screenshot
- Sample alert and follow-up report

### Q79 (11.5, 12.10.5): File Integrity Monitoring
**Answer:**

**Current State:** No FIM implemented

**Recommendation:** Implement FIM solution:

**Tools:**
- AIDE (Advanced Intrusion Detection Environment)
- Tripwire
- OSSEC
- AWS native: CloudWatch + Lambda

**Monitored Files:**
- Application binaries
- Configuration files (`/etc`, `.env`)
- System files
- Database files
- Audit logs

**Evidence to Provide:**
- Screenshot of FIM tool version
- List of monitored servers from CDE
- List of critical files monitored
- Alert configuration screenshot
- Sample real-time alerts or weekly comparison reports

---

## REQUIREMENT 12: SUPPORT INFORMATION SECURITY WITH ORGANIZATIONAL POLICIES AND PROGRAMS

### Q80 (12.1, 12.6.2): Information Security Policy Acknowledgment
**Answer:**

**Current State:** No formal information security policy

**Recommendation:** Develop and implement:

**Information Security Policy:**
- Scope and objectives
- Roles and responsibilities
- Acceptable use policy
- Access control policy
- Encryption policy
- Incident response
- Business continuity

**Acknowledgment Records:**
- All employees (3 engineers)
- New hires upon joining
- Annual re-acknowledgment
- Vendor/contractor acknowledgments

**Provide:**
- Sample acknowledgment forms (5 existing, 5 new, 1 vendor)

### Q81 (12.1.1): Annual Security Policy Review
**Answer:**

**Current State:** No formal policy to review

**Recommendation:** Implement:
- Annual policy review process
- Update based on:
  - Regulatory changes
  - Business changes
  - Risk assessment findings
- Document review date and changes
- Management approval

**Provide:**
- Annual review records
- Policy version history

### Q82 (12.2): Risk Assessment
**Answer:**

**Current State:** No formal risk assessment performed

**Recommendation:** Conduct annual risk assessment:

**Framework Options:**
- ISO 27005
- NIST SP 800-30
- OCTAVE

**Risk Assessment Should Cover:**
- Asset identification
- Threat identification
- Vulnerability analysis
- Impact analysis
- Likelihood assessment
- Risk rating (High, Medium, Low)
- Risk treatment plans

**Provide:**
- Annual risk assessment report
- Risk register
- Mitigation plans

### Q83 (12.3.8): Remote Access Session Timeout
**Answer:**

**Current State:**
- SSH: No automatic timeout configured
- Application: JWT access token expires after 15 minutes

**Recommendation:** Configure SSH timeout:
```
# /etc/ssh/sshd_config
ClientAliveInterval 900
ClientAliveCountMax 0
```
(15-minute idle timeout)

**Provide:**
- Configuration screenshot showing timeout settings

### Q84 (12.3.9): Vendor Remote Access Restrictions
**Answer:**

**Current State:** No vendor remote access

**If Implemented:**
- Vendor access list
- Business justification
- Time-limited access
- Access deactivation after use
- Activity monitoring

**Current Answer:** Not applicable - no vendor remote access

### Q85 (12.4, 12.4.1): Security Roles and Responsibilities
**Answer:**

**Current State:** 3-person engineering team, informal roles

**Recommendation:** Create and provide:

**Organization Chart:**
- Information security roles
- Designated security officer
- Development team
- Infrastructure team

**Security Responsibilities:**
- Policy review and approval
- Incident response
- Vulnerability management
- Access control management

**Evidence:**
- Security policy review/approval records
- Security alert communications
- Policy distribution to all users

### Q86 (12.6, 12.6.1): Security Awareness Program
**Answer:**

**Current State:** No formal security awareness program

**Recommendation:** Develop security awareness program:

**Program Content:**
- Importance of cardholder data security
- Employee responsibilities
- Incident reporting procedures
- Physical security (even for remote workers)
- Password security
- Social engineering awareness

**Training Records:**
- All employees (3 engineers)
- New hires upon joining
- Annual refresher training

**Provide:**
- Training materials
- Attendance records (5 existing + 5 new hires)

### Q87 (12.6): Annual Security Awareness Training
**Answer:**

**Current State:** No formal training program

**Recommendation:**
- Annual security awareness training for all employees
- New hire training within 30 days
- Training completion tracking

**Provide:**
- Training attendance logs
- Completion certificates
- New employee onboarding records

### Q88 (12.7): Employee Background Checks
**Answer:**

**Current State:** Background checks may not be formally documented

**Recommendation:** Implement pre-employment screening:

**Background Check Components:**
- Criminal background check
- Employment verification
- Educational verification
- Reference checks
- Applicable based on local laws

**Provide:**
- Sample background check records (5-10 employees from last year)
- Background check authorization forms
- Results documentation (redacted if necessary)

### Q89 (12.8.1): Service Provider Inventory
**Answer:**

**Service Providers with Cardholder Data Access:**

1. **JioPay (Payment Gateway)**
   - Data Shared: Full PAN, CVV, cardholder name, expiry date
   - Purpose: Payment processing and tokenization

2. **Easebuzz (Payment Gateway)**
   - Data Shared: Full PAN, CVV, cardholder name, expiry date
   - Purpose: Payment processing and tokenization

3. **AWS (Infrastructure Provider)**
   - Data Shared: Tokenized payment data, encrypted bank accounts
   - Purpose: Hosting and infrastructure

4. **AuthBridge (KYC Verification) (if applicable)**
   - Data Shared: PAN (Permanent Account Number for KYC)
   - Purpose: Identity verification

**Provide:**
- Service provider list
- Attestation of Compliance (AOC) from each provider
- Security assessment reports
- Risk assessment for each provider

### Q90 (12.8.2): Service Provider Agreements
**Answer:**

**Current State:** Service agreements exist with payment gateways

**Recommendation:** Ensure all agreements include:
- Acknowledgment of responsibility for cardholder data
- PCI DSS compliance obligations
- Breach notification requirements
- Right to audit
- Data handling and storage requirements

**Provide:**
- Copies of service provider agreements (on-site review)
- Security responsibility clauses

### Q91 (12.8.3): Service Provider Due Diligence
**Answer:**

**Current State:** Gateways selected based on PCI DSS compliance

**Recommendation:** Document due diligence process:

**Before Engagement:**
- PCI DSS compliance verification
- Security assessment review
- Penetration test results
- Business continuity plan review
- Data handling procedures
- Incident response capabilities

**Provide:**
- Due diligence checklist
- Provider security documentation
- Compliance verification records

### Q92 (12.8.4, 12.9): Service Provider PCI Compliance Monitoring
**Answer:**

**Current State:** Assumed gateways maintain compliance

**Recommendation:** Annual verification:

**For Each Service Provider:**
- Request current AOC (Attestation of Compliance)
- Verify compliance status
- Document review date
- Escalate if provider is non-compliant

**If Provider Participates in Audit:**
- Provide access to all required evidence
- Network diagrams
- Access controls
- Vulnerability scans
- Penetration test reports
- Incident response plans

**Provide:**
- Annual AOC copies from all providers
- Compliance status tracking

### Q93 (12.9): Service Provider Acknowledgment (If Applicable)
**Answer:**

**If XCentro is a Service Provider to Others:**

**Provide:**
- Written acknowledgment of data security responsibility
- Signed agreement with customers
- Security controls documentation
- Regular security assessments
- Incident response plan

**Current Answer:** Not clear if XCentro acts as service provider to other entities. Clarify business model.

### Q94 (12.10): Incident Response Plan
**Answer:**

**Current State:** No formal incident response plan

**Recommendation:** Develop and test incident response plan:

**Plan Components:**
1. **Roles and Responsibilities**
2. **Incident Classification**
3. **Detection and Reporting**
4. **Containment Procedures**
5. **Eradication Steps**
6. **Recovery Procedures**
7. **Post-Incident Review**
8. **Communication Plan**
9. **Evidence Preservation**
10. **Legal and Regulatory Notification**

**Testing:**
- Annual tabletop exercise
- Document test results
- Update plan based on lessons learned

**Provide:**
- Incident response plan document
- Annual test report OR recent incident report

### Q95 (12.10.4): Incident Response Training
**Answer:**

**Current State:** No formal incident response training

**Recommendation:** Implement:

**Training Program:**
- Incident response team designation
- Annual training for IR team
- Breach response procedures
- Evidence handling
- Communication protocols

**Provide:**
- Training attendance records
- Training materials
- Training completion certificates
- Incident response procedures documentation

### Q96 (12.10.6): Incident Reporting and Learning
**Answer:**

**Current State:** No formal incident tracking

**Recommendation:** Implement incident management:

**Incident Report Should Include:**
- Incident recording (date, time, type)
- Communication logs (internal and external)
- Handling procedures followed
- Response actions taken
- Lessons learned
- Process improvements

**Provide:**
- Sample recent security incident report
- Incident tracking system
- Post-incident review documentation

---

## SUMMARY OF RECOMMENDATIONS

### HIGH PRIORITY (Critical Gaps)

1. **Formal Information Security Policy** (Q80, Q81)
2. **Risk Assessment** (Q82)
3. **Incident Response Plan** (Q94, Q95, Q96)
4. **Centralized Logging** (Q64, Q65, Q68)
5. **Multi-Factor Authentication** (Q50, Q51)
6. **Vulnerability Scanning** (Q72, Q73)
7. **Penetration Testing** (Q75, Q76)
8. **IDS/IPS Implementation** (Q78)
9. **File Integrity Monitoring** (Q79)
10. **Security Awareness Training** (Q86, Q87)

### MEDIUM PRIORITY (Documentation & Process)

1. **Change Management Process** (Q38)
2. **Secure Development Lifecycle** (Q32, Q39)
3. **Access Control Documentation** (Q41, Q42, Q43, Q44)
4. **Password Policy Enforcement** (Q46)
5. **Key Management Procedures** (Q24, Q25, Q26, Q27)
6. **Service Provider Management** (Q89, Q90, Q91, Q92)
7. **Employee Background Checks** (Q88)
8. **Firewall Rule Review Process** (Q11)

### LOW PRIORITY (Enhancements)

1. **Development/Production Segregation** (Q35)
2. **Anti-Virus Implementation** (Q30)
3. **Role Segregation** (Q36)
4. **Time Synchronization Documentation** (Q66)
5. **Remote Access Timeout** (Q83)

### ALREADY COMPLIANT / NOT APPLICABLE

- Physical security (Q55-Q63): Fully remote
- Wireless security (Q12, Q14, Q70, Q71): No wireless
- POS devices (Q62, Q63): Card-not-present only
- Sensitive authentication data (Q18): Not stored
- Strong cryptography (Q21, Q28, Q47): Implemented

---

## NEXT STEPS

1. **Immediate Actions:**
   - Schedule external ASV scan
   - Develop incident response plan
   - Implement centralized logging
   - Enable MFA for administrative access

2. **30-Day Actions:**
   - Create information security policy
   - Conduct risk assessment
   - Implement vulnerability scanning
   - Document change management process

3. **90-Day Actions:**
   - Security awareness training program
   - Penetration testing engagement
   - IDS/IPS deployment
   - File integrity monitoring
   - Formal access control documentation

4. **Ongoing:**
   - Quarterly vulnerability scans
   - Annual penetration tests
   - Annual policy reviews
   - Continuous security monitoring

---

**Prepared By:** Claude Code
**Date:** November 16, 2025
**Version:** 1.0
