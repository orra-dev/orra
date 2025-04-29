# Telemetry

To help us improve orra, we collect minimal, privacy-preserving usage analytics. **Telemetry is anonymous by design, and you can easily opt out.**

## What We Collect

- **[Anonymous usage events](../planengine/events.go)** (e.g., server start/stop, project/service changes, execution plan outcomes).
- **Non-identifiable execution plan and project tracking:**
    - For events related to projects, we only track a **hashed version of the project ID**.
    - For execution plan events, we only track a **hashed version of the execution plan ID**.
- **No personal or sensitive data** is ever collected.
- **No IP addresses** or environment details are sent.

## How IDs Are Handled

- We use a **SHA-256 hash** of each project or execution plan ID.  
  This ensures IDs cannot be reversed or linked to your actual data.

- Each orra instance also generates a random, anonymous identifier stored locally, used solely to distinguish unique installations.

## How to Opt Out

You can fully disable telemetry at any time:

- **Via environment variable:**
  Set the environment variable before starting orra:
  ```shell
  export ANONYMIZED_TELEMETRY=false
  ```
- **Via `.env` file:**  
  Add the following line to your project's `.env` file:
  ```text
  ANONYMIZED_TELEMETRY=false
  ```

## Transparency and Trust

- All telemetry code is open source and can be reviewed [here](../planengine/telemetry.go).
- We follow industry best practices for privacy and transparency.
- Telemetry helps us prioritize features, fix bugs, and ensure orra works reliably for everyone.

**Thank you for helping us make orra better!**
