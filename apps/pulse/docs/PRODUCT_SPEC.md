# Pulse — Master Product & Implementation Specification

> Verbatim product specification as given. This is the source of truth for product intent; `ARCHITECTURE_AUDIT.md` maps it onto the real Core Platform and `docs/control/pulse.html` tracks build status against it.

## 1. Project Context

You are implementing a new product called **Pulse** on top of an existing reusable **Core Platform**.

The Core Platform already provides reusable platform capabilities including:

* Identity
* Authentication
* User management
* Profiles
* Devices
* Sessions
* Authorization
* Relationships
* Groups / circles
* Realtime infrastructure
* Messaging infrastructure
* Notifications
* Push notifications
* Email
* Files/media
* Search
* Events
* Background jobs
* Workflows
* Feature flags
* Remote configuration
* Audit
* Privacy
* Blocking
* Reporting
* Moderation
* Billing
* Entitlements
* Analytics
* Observability
* Admin tooling
* SDKs
* Infrastructure
* CI/CD

**Do not recreate Core Platform functionality inside Pulse.**

Pulse must consume Core APIs, SDKs, events and extension mechanisms.

The architectural dependency must always remain:

```text
Pulse
  ↓
Core Platform
```

Never:

```text
Core Platform
  ↓
Pulse
```

Core must never acquire Pulse-specific concepts.

---

## 2. Product Vision

Pulse is a **global non-verbal social communication application**.

Its purpose is:

> Allow people who care about each other to communicate presence, emotion and attention without needing to type or speak.

The defining principle is:

> **Feel it instead of reading it.**

Pulse must NOT become another WhatsApp, Instagram, Snapchat or conventional messaging application.

Text communication is not the centre of the product.

The product centres around:

```text
Touch
Presence
Mood
Signals
Haptics
Relationships
Shared moments
```

The primary interaction is physical/emotional rather than textual.

---

## 3. Original Product Idea

The original interaction is:

> When someone misses another person, they press and hold a button.

While they hold:

```text
Sender presses
      ↓
Pulse starts
      ↓
Receiver experiences haptic feedback
      ↓
Sender continues holding
      ↓
Receiver continues feeling the Pulse where OS/app state permits
      ↓
Sender releases
      ↓
Pulse ends
```

The duration of the sender's hold represents the duration of the emotional gesture.

Example:

```text
Sender holds for 4.8 seconds
```

The system records:

```text
duration = 4800 ms
```

When both devices are actively connected, the receiving device should reproduce the live interaction as closely as technically possible.

---

## 4. Important OS Reality

iOS and Android have different background/haptic restrictions.

Never promise exact continuous remote vibration while an iPhone app is backgrounded or the phone is locked if iOS does not permit it.

Implement two experiences.

### 4.1 Live Pulse

When receiver is:

```text
online
+
app active
+
realtime session available
```

use realtime communication.

```text
PulseStart
     ↓
Receiver starts native haptic

PulseStop
     ↓
Receiver stops native haptic
```

This should approximate the sender's actual hold duration.

---

### 4.2 Offline / Background Pulse

When receiver is not available for Live Pulse:

```text
Sender completes Pulse
       ↓
Pulse stored
       ↓
Push notification generated
       ↓
Receiver receives platform-appropriate notification/haptic
```

The application may later reproduce/show the Pulse duration when opened.

Do NOT attempt unsupported background behaviour.

---

## 5. Product Positioning

Pulse should not be marketed narrowly as:

> "I miss you app."

That was only the starting interaction.

The broader product is:

> **A private non-verbal communication network for the people who matter to you.**

Possible positioning:

> Feel someone thinking about you.

or:

> Say something without saying anything.

or:

> A social network you feel instead of read.

Brand language can evolve.

---

## 6. Relationship Model

Pulse is NOT exclusively for couples.

A person can have:

```text
0 or 1 romantic Partner
+
multiple Close Friends
+
multiple Friends
+
custom Circles
```

The one-Partner constraint should be product policy/configurable rather than embedded into Core.

Pulse relationship terminology:

```text
Partner / Bond
Close Friend
Friend
Circle
Connection
```

Recommended conceptual naming:

```text
Bond
```

for the special romantic connection.

---

## 7. Partner Bond

A user may optionally establish **one active romantic Bond**.

Example:

```text
User A
 ↕
User B

Relationship type:
PARTNER_BOND
```

A Bond requires explicit mutual acceptance.

Never establish a Bond automatically.

Possible states:

```text
PENDING
ACTIVE
ENDED
BLOCKED
```

Partner-specific experiences may include:

```text
Live Touch
Long Pulse
Private touch language
Intimate mood visibility
Shared moments
Special animations
Future wearable interactions
```

---

## 8. Friends

Users can establish regular friend connections.

Friends can participate in less intimate forms of interaction:

```text
Pulse
Knock
Mood
Reactions
Circles
```

Exact permissions should be controlled through Pulse product policy.

---

## 9. Close Friends

A user may classify selected friends as a **Close Circle** or **Close Friends**.

Close friends may receive:

```text
more private moods
additional signal types
closer presence interactions
```

Do not assume everyone sees everything.

---

## 10. Circles

Pulse should support custom groups/circles.

Examples:

```text
Closest Friends
Family
University
Work Friends
Gaming Friends
Custom
```

Circle membership is private unless product requirements explicitly make it visible.

Circles primarily control:

```text
mood audience
signal permissions
future group interactions
```

---

## 11. Connection Flow

Users must mutually connect before direct interactions are possible.

Possible invitation methods:

```text
Username
QR code
Invite link
Contact discovery
Deep link
Share code
```

Initial flow:

```text
User A sends request
        ↓
User B receives request
        ↓
Accept / Decline
        ↓
Connection becomes active
```

For Partner Bond:

```text
Existing connection
      ↓
Request Bond
      ↓
Explicit confirmation
      ↓
Active Bond
```

---

## 12. Main Product Primitives

The app should be designed around three fundamental concepts:

```text
PEOPLE
STATE
INTERACTION
```

### PEOPLE

```text
Partner
Friends
Close Friends
Circles
```

### STATE

```text
Today's Mood
Presence
Availability
```

### INTERACTION

```text
Pulse
Pulse Back
Knock
Live Touch
Custom Signals
Future gestures
```

Every major feature should fit naturally into one of these categories.

---

## 13. Pulse

A **Pulse** is the fundamental interaction.

### Sender UX

The main screen should contain a large interaction area.

Example concept:

```text
        ❤️

      Rachel


     ┌─────────┐
     │         │
     │  HOLD   │
     │    ♥    │
     │         │
     └─────────┘

    Hold to Pulse
```

While holding:

```text
0.4s
1.2s
2.8s
4.7s
```

The visual should react continuously.

Possible behaviour:

```text
heart expands
glow intensifies
subtle sender haptic
animation breathes
```

---

## 14. Pulse Duration

Configurable product limits should exist.

Suggested initial configuration:

```text
minimum valid pulse: 150 ms

normal maximum:
10–15 seconds

rate limit:
product-configured
```

Do not hardcode these values.

Use Core Remote Configuration.

---

## 15. Pulse Network Protocol

Do NOT send continuous events every few milliseconds.

Use:

```text
PulseStart
```

and:

```text
PulseStop
```

Example:

```json
{
  "interactionId": "...",
  "senderId": "...",
  "receiverId": "...",
  "startedAt": "..."
}
```

Then:

```json
{
  "interactionId": "...",
  "endedAt": "..."
}
```

The server calculates or validates duration.

Do not fully trust client-submitted duration.

---

## 16. Pulse State Machine

Possible states:

```text
CREATED
STARTED
LIVE_DELIVERED
COMPLETED
PUSH_REQUESTED
PUSH_SENT
OPENED
EXPIRED
FAILED
```

Not every Pulse must pass through every state.

---

## 17. Pulse Back

After receiving a Pulse, the recipient may respond instantly.

Primary action:

```text
Pulse Back
```

It should be faster than opening a messaging interface.

Potential flow:

```text
A → Pulse
B → feels it
B → Pulse Back
A → feels response
```

This creates the fundamental social loop.

---

## 18. Knock

**Knock** is a short non-verbal attention gesture.

Think of knocking on someone's digital door.

Example:

```text
tap tap
```

Receiver feels:

```text
buzz buzz
```

No sentence needs to appear.

Knock should support multiple patterns later.

Examples:

```text
••

•••

• — •

— • —
```

---

## 19. Private Touch Language

This is an important long-term differentiator.

Users should eventually be able to communicate using patterns whose meaning exists only between them.

Example:

```text
••
```

could mean:

```text
I love you
```

for one pair.

For another pair:

```text
••
```

might mean:

```text
I'm home
```

The platform does not need to understand the semantic meaning.

The pattern itself is the communication.

Potential signal components:

```text
short tap
long hold
pause
double tap
triple tap
tap + hold
custom rhythm
```

---

## 20. Custom Signal Storage

A custom signal may conceptually contain:

```text
SignalPattern
----------------
id
owner/context
segments[]
visibility
created_at
```

Segment example:

```json
[
  { "type": "tap", "duration": 150 },
  { "type": "pause", "duration": 300 },
  { "type": "hold", "duration": 900 }
]
```

Do not interpret custom semantics server-side unless future requirements explicitly require optional labels.

---

## 21. Live Touch

Live Touch should be one of Pulse's flagship features.

When two users are simultaneously active:

```text
User A touches
       ↓
User B feels it

User B touches
       ↓
User A feels it
```

Possible mode:

```text
Live Touch Session
```

Flow:

```text
Invite/enter live session
        ↓
Both connected to Realtime
        ↓
Touch start
        ↓
Remote haptic start
        ↓
Touch release
        ↓
Remote haptic stop
```

Potential visual:

```text
two glowing circles
two hearts
touch points
breathing animation
```

Keep latency as low as reasonably possible.

---

## 22. Today's Mood

**Today's Mood** is the passive emotional state layer.

It should answer:

> How am I feeling today?

without requiring a status sentence.

Primary interaction should be visual/non-verbal.

Examples:

```text
☀️
🌧️
🌙
🔥
🌊
🫂
❤️
💤
```

Possible conceptual meanings:

```text
happy
low
quiet
energetic
overwhelmed
need closeness
affectionate
tired
```

Do not necessarily display these meanings after onboarding.

---

## 23. Mood Design Philosophy

Mood should NOT become:

```text
"Feeling depressed because work was difficult today..."
```

Pulse is intentionally non-verbal.

The desired experience:

```text
User selects symbol/visual
        ↓
Selected audience sees visual
```

No obligation to explain.

---

## 24. Mood Visual System

Long-term, consider moving beyond ordinary emojis.

Mood could be represented using:

```text
colour
motion
brightness
particle behaviour
shape
animation speed
breathing rhythm
```

Examples:

```text
slow breathing orb
= calm

rapid glowing orb
= excited

dim drifting orb
= low

two overlapping shapes
= wants closeness
```

This could become part of Pulse's unique visual identity.

---

## 25. Mood Audience

Every Mood must have visibility.

Examples:

```text
PARTNER_ONLY
CLOSE_FRIENDS
SELECTED_CIRCLES
ALL_CONNECTIONS
CUSTOM_USERS
PRIVATE
```

Never assume all friends should see every Mood.

Use Core authorization/relationship capabilities.

---

## 26. Mood Expiry

Today's Mood is temporary state.

Suggested behaviour:

```text
expires at user-local day boundary
```

or configurable duration.

Store timezone correctly.

Do not keep stale moods indefinitely as active state.

Historical Mood storage is a separate product decision.

---

## 27. Responding to Mood

A user sees another person's Mood and can respond without words.

Potential responses:

```text
Pulse
Knock
Hug signal
Heart signal
Spark
Custom touch
```

Do not immediately introduce text replies.

Example:

```text
Rachel: 🌧️

Azariah sees it
      ↓
sends 🫂 haptic
```

No conversation required.

---

## 28. Non-Verbal Status

Avoid conventional social presence such as:

```text
last seen 7:14 PM
online 2 minutes ago
```

unless there is a compelling privacy-safe reason.

Prefer ambient indicators:

```text
glowing heart
soft orb
presence glow
available-for-live-touch symbol
```

Avoid surveillance behaviour.

---

## 29. Available for Touch

Optional future presence state:

```text
Available for Live Touch
```

This should be:

```text
opt-in
temporary
privacy-controlled
```

Never expose detailed device/activity information.

---

## 30. Shared Moments

Pulse may maintain a private timeline of meaningful interactions.

Example:

```text
8:42 PM

You both Pulsed each other.
```

or:

```text
Shared Touch
12 seconds
```

Avoid turning this into chat history.

---

## 31. Save This Moment

Potential interaction:

```text
Save this moment ♥
```

This saves:

```text
timestamp
participants
interaction type
duration/pattern
```

without storing unnecessary content.

---

## 32. No Streak Pressure

Do NOT create manipulative mechanics such as:

```text
You lost your 148-day love streak!
```

Pulse should create closeness, not obligation.

If statistics are shown, make them gentle.

Example:

```text
12 little moments together this week.
```

rather than:

```text
YOU FAILED YOUR STREAK
```

---

## 33. Quiet Hours

Users should be able to configure quiet periods.

Example:

```text
11:00 PM – 7:00 AM
```

During quiet hours:

```text
Pulse may be stored
notification may be silent
recipient sees it later
```

Respect system notification settings.

Use Core notification preferences.

---

## 34. Muting

Allow:

```text
Mute 1 hour
Mute tonight
Mute until tomorrow
Mute indefinitely
```

Mute behaviour must be distinct from Block.

---

## 35. Blocking

Users must be able to block another user.

Blocked users cannot:

```text
Pulse
Knock
start Live Touch
view restricted Mood
send connection requests where policy prevents it
```

Enforce server-side.

Do not rely on UI-only blocking.

Use Core blocking system.

---

## 36. Abuse Protection

Pulse interactions could be abused.

Example:

```text
500 Knocks
```

therefore implement:

```text
per-user limits
per-target limits
device limits
burst limits
cooldowns
abuse detection
mute
block
report
```

Use Core Trust & Safety capabilities.

---

## 37. Rate Limiting

Rate limits must be remote-configurable.

Potential dimensions:

```text
user
receiver
device
IP
interaction type
time window
```

Never hardcode one global limit into business logic.

---

## 38. Reporting

Users must be able to report:

```text
user
interaction abuse
harassment
spam
impersonation
other
```

Use Core reporting/moderation infrastructure.

---

## 39. Privacy

Pulse must be privacy-first.

Avoid building:

```text
precise location tracking
constant last-seen surveillance
secret online indicators
relationship monitoring
read-pressure mechanics
```

Only collect data required for product functionality.

---

## 40. Location

Pulse does not require GPS for its fundamental features.

Timezone may be used for:

```text
Mood expiry
Quiet Hours
scheduled interactions
```

If future location features are introduced, require explicit opt-in.

---

## 41. Scheduled Pulse

Potential future premium feature:

```text
record/prepare a Pulse
      ↓
deliver at chosen future time
```

Example:

```text
send when partner wakes up
```

Use Core workflow/scheduler.

Do not implement custom scheduling engine inside Pulse.

---

## 42. Wearables

Future expansion:

```text
Apple Watch
Wear OS
```

Wearable experience may become one of Pulse's strongest capabilities because touch/haptic communication can feel more natural on the wrist.

Do not make initial product architecture dependent on wearable availability.

---

## 43. Widgets / Quick Access

Future:

```text
iOS widget
Android widget
lock-screen shortcut
home-screen shortcut
watch complication
```

Goal:

> Send a Pulse with minimal navigation.

---

## 44. Product Navigation

Recommended primary navigation:

```text
Home
People
Mood
Moments
Profile
```

Avoid excessive navigation.

Possible Home:

```text
Partner / selected person
Mood indicators
large Pulse control
recent incoming signal
quick people switcher
```

---

## 45. Home Experience

Home should feel emotional and visually calm.

Avoid:

```text
dense feeds
advertisement-like cards
large text walls
engagement spam
```

Pulse should feel more like a shared space than a conventional social feed.

---

## 46. People Screen

Show:

```text
Partner Bond
Close Friends
Friends
Circles
Pending Requests
```

Allow search/add/invite.

---

## 47. Mood Screen

Provide:

```text
current Mood
Mood selector
audience
expiration
friends' visible Moods
```

Mood display should prioritize visuals over labels.

---

## 48. Moments Screen

Potentially show:

```text
recent Pulses
shared Touch
saved moments
meaningful signal exchanges
```

Keep privacy and retention configurable.

---

## 49. Profile

Profile may contain:

```text
avatar
display name
username
relationship controls
privacy
notification settings
quiet hours
devices
blocked users
subscription
account
```

Do not over-socialize profiles initially.

---

## 50. Onboarding

Onboarding should rapidly demonstrate the product.

Potential sequence:

```text
Welcome
↓
Create/sign into account
↓
Choose username/profile
↓
Find/connect someone
↓
Send first Pulse
```

The first successful Pulse should occur as early as possible.

---

## 51. Viral Invitation Loop

Pulse naturally requires other people.

Invitation should therefore be a core growth mechanism.

Avoid generic:

```text
John invited you to Pulse.
```

Prefer emotionally relevant but respectful language.

Example concept:

```text
Azariah wants to connect with you on Pulse.
```

For Bond:

```text
Azariah invited you to create a Bond.
```

Do not send manipulative invitations.

---

## 52. Deep Linking

Invitation links must support:

```text
app installed
app not installed
iOS
Android
web fallback
```

Use Core deep-link infrastructure if available.

---

## 53. Authentication

Use Core Identity.

Pulse must NOT implement separate authentication.

Support whatever providers Core makes available, potentially:

```text
Apple
Google
email
phone
passkeys
```

Pulse receives Core User identity.

---

## 54. User Model

Do not duplicate Core users.

Pulse-specific profile extension may exist.

Concept:

```text
PulseProfile
----------------
user_id
username/public handle if Pulse-specific
visual preferences
Pulse preferences
created_at
```

Only store fields actually specific to Pulse.

---

## 55. Relationship Storage

Use Core generic relationship system where possible.

Pulse-specific relationship metadata may contain:

```text
relationship type
Pulse permissions
Bond metadata
interaction preferences
```

Do not duplicate entire Core relationship records unnecessarily.

---

## 56. Domain Modules

Recommended Pulse product modules:

```text
pulse-profile

pulse-connections

bond

mood

pulse-interactions

signals

live-touch

moments

pulse-preferences

pulse-entitlements
```

Do not mix all functionality into one giant service.

---

## 57. Deployment Architecture

Initially Pulse-specific backend logic can exist as a modular product service/application on Core infrastructure.

Concept:

```text
Mobile App
     ↓
Core API Gateway
     ↓
Pulse Product API
     ↓
Core Services
```

Realtime:

```text
Mobile
   ↓
Core Realtime Gateway
   ↓
Pulse realtime handlers
```

Do not create a separate realtime infrastructure if Core already provides it.

---

## 58. Mobile Technology

Use the platform's established mobile technology.

Current expected direction:

```text
Flutter
```

Use native integration where needed:

```text
Swift
    Core Haptics
    iOS notification integration

Kotlin
    Android vibration/haptic integration
```

Do not compromise the flagship haptic experience merely to remain 100% Dart-only.

---

## 59. Haptic Abstraction

Create a Pulse haptic abstraction.

Concept:

```text
HapticEngine
```

Operations:

```text
playPulseStart()
playPulseStop()
playPattern()
playKnock()
playMoodResponse()
supportsAdvancedHaptics()
```

Implement platform adapters:

```text
IOSHapticEngine
AndroidHapticEngine
```

---

## 60. Graceful Haptic Degradation

Different devices have different hardware.

Support capability detection.

Possible levels:

```text
ADVANCED
STANDARD
BASIC
UNAVAILABLE
```

Product should still work even if advanced haptics are unavailable.

---

## 61. Realtime Events

Potential Pulse realtime events:

```text
pulse.started
pulse.stopped

knock.sent

live_touch.invited
live_touch.accepted
live_touch.started
live_touch.touch_started
live_touch.touch_stopped
live_touch.ended

presence.changed

signal.started
signal.segment
signal.completed
```

Do not necessarily persist ephemeral events.

---

## 62. Durable Domain Events

Potential durable Pulse events:

```text
pulse.completed
pulse.delivered

bond.requested
bond.accepted
bond.ended

mood.updated
mood.expired

moment.saved

signal.created

interaction.reported
```

Publish through Core Event infrastructure.

---

## 63. Pulse API Concepts

Potential REST endpoints:

```text
POST /v1/pulse/interactions
GET  /v1/pulse/interactions/{id}

POST /v1/pulse/knocks

GET  /v1/pulse/moods
PUT  /v1/pulse/moods/me
DELETE /v1/pulse/moods/me

GET  /v1/pulse/moments
POST /v1/pulse/moments/{interactionId}/save

GET  /v1/pulse/bond
POST /v1/pulse/bond/requests
POST /v1/pulse/bond/requests/{id}/accept
POST /v1/pulse/bond/requests/{id}/decline
DELETE /v1/pulse/bond
```

Exact endpoint design must follow existing Core API conventions.

Use contract-first OpenAPI.

---

## 64. Interaction Entity

Conceptual model:

```text
Interaction
-------------------------
id
type
sender_user_id
receiver_user_id
relationship_context
started_at
ended_at
duration_ms
delivery_mode
status
created_at
```

Potential types:

```text
PULSE
KNOCK
CUSTOM_SIGNAL
LIVE_TOUCH
MOOD_RESPONSE
```

Avoid storing excessive ephemeral detail unnecessarily.

---

## 65. Pulse Data Model

Concept:

```text
Pulse
----------------
interaction_id
started_at
ended_at
duration_ms
delivery_mode
```

Delivery mode:

```text
LIVE
PUSH
DEFERRED
```

---

## 66. Mood Data Model

Concept:

```text
Mood
----------------
id
user_id
mood_visual_id
audience_type
created_at
expires_at
```

Audience mapping may exist separately.

---

## 67. Bond Data Model

If Pulse-specific metadata is necessary:

```text
Bond
----------------
id
core_relationship_id
status
requested_at
accepted_at
ended_at
```

Do not duplicate Core users.

---

## 68. Moment Data Model

Concept:

```text
Moment
----------------
id
owner/context
interaction_id
created_at
```

Store references rather than unnecessary duplicate payload.

---

## 69. Custom Signal Model

Concept:

```text
CustomSignal
----------------
id
creator/context
pattern_version
pattern
created_at
```

Potential relationship-scoped signals should not leak between unrelated users.

---

## 70. Presence

Use Core Presence/Realtime.

Pulse may define product-specific states such as:

```text
AVAILABLE_FOR_TOUCH
IN_LIVE_TOUCH
```

but avoid exposing exact user activity where unnecessary.

---

## 71. Push Notifications

Use Core Notification Service.

Pulse must request semantic notifications.

Example:

```text
category:
PULSE_RECEIVED
```

Core selects:

```text
FCM
APNs
quiet-hours behaviour
device routing
retry
```

Pulse must not call APNs/FCM directly.

---

## 72. Notification Content

Provide privacy controls.

Possible user setting:

```text
Detailed:
"Azariah sent you a Pulse"

Private:
"New Pulse"

Silent:
no visible text
```

Consider lock-screen privacy.

---

## 73. Multiple Devices

A Core user may have multiple devices.

Pulse needs policy for:

```text
send to all active devices
primary device only
wearable preferred
current active device
```

Make this configurable.

Avoid generating chaotic simultaneous vibration across every device by default.

---

## 74. Delivery Routing

Conceptual priority:

```text
active live session
    ↓
active foreground device
    ↓
preferred wearable/device
    ↓
push notification
```

Use Core device/realtime infrastructure.

---

## 75. Reliability

Important interactions should have IDs and acknowledgment where appropriate.

Do not make Live Touch wait for database persistence.

Realtime critical path:

```text
sender
↓
gateway
↓
receiver
```

Persistence/analytics may happen asynchronously.

---

## 76. Idempotency

Pulse creation and durable interaction completion must support idempotency.

Mobile networks retry.

Do not create duplicate Pulses from the same request.

---

## 77. Offline Behaviour

If sender temporarily loses connection:

```text
detect disconnection
close local interaction
attempt durable completion
show honest state
```

Do not falsely display delivered if delivery is unknown.

---

## 78. Clock Handling

Use server timestamps for durable truth.

Client timestamps may be included for UX/latency analysis but should not be blindly trusted.

Store UTC.

Render according to user timezone.

---

## 79. Analytics

Use Core Analytics.

Important events may include:

```text
onboarding_started
onboarding_completed

connection_invited
connection_accepted

bond_requested
bond_created

pulse_started
pulse_completed
pulse_received
pulse_back

knock_sent

mood_set
mood_viewed
mood_responded

live_touch_started
live_touch_completed

moment_saved
```

Avoid collecting unnecessary personal content.

---

## 80. Key Product Metrics

The most important metrics are not raw downloads.

Track:

```text
connected users
connected pairs
first Pulse success
time to first Pulse

D1 retention
D7 retention
D30 retention

Pulses per connected pair
reciprocal Pulse rate
Pulse Back rate

Mood creation rate
Mood response rate

Live Touch adoption
Live Touch completion

connection invitation conversion
```

---

## 81. Core North-Star Candidate

Potential:

```text
Weekly Connected Pairs with Reciprocal Interaction
```

Meaning:

> Two connected people both intentionally interacted with each other during the week.

This is more meaningful than:

```text
app opens
```

---

## 82. Retention Question

The critical product question is:

> Does Pulse remain emotionally useful after the novelty disappears?

Design analytics to answer this.

Avoid artificially manufacturing retention through intrusive notifications.

---

## 83. Monetisation

Core emotional communication should remain free.

Potential future premium:

```text
advanced custom haptics
special visual themes
scheduled Pulse
longer Moments history
premium Bond customization
wearable enhancements
custom signal packs
couple/shared themes
advanced personalization
```

Do not put basic affection behind a paywall.

---

## 84. Billing

Use Core Billing and Entitlements.

Pulse should ask:

```text
hasEntitlement("pulse.plus")
```

Do not directly couple features to Stripe/Apple/Google billing implementation.

---

## 85. Internationalization

Pulse is intended to be global.

Support:

```text
multiple languages
RTL layouts eventually
timezone
locale
regional formatting
accessibility
```

Even though much of the app is non-verbal, onboarding/settings/notifications still need localization.

---

## 86. Accessibility

Do not assume every user can perceive haptics equally.

Provide alternatives:

```text
visual animation
flash/glow where appropriate
sound optional
larger visual cues
reduced motion
high contrast
screen reader labels
```

Respect OS accessibility settings.

---

## 87. Battery Usage

Realtime must not unnecessarily drain battery.

Use:

```text
efficient WebSocket lifecycle
OS push when backgrounded
reasonable heartbeat intervals
foreground realtime optimization
```

Do not keep aggressive realtime loops alive unnecessarily.

---

## 88. Network Efficiency

For Pulse:

```text
START
STOP
```

not hundreds of continuous packets.

For complex signals, send compact pattern structures where appropriate.

---

## 89. Security

Every interaction must be authorized server-side.

Never accept:

```text
receiverId = X
```

and assume sender may contact X.

Validate:

```text
connection exists
relationship permits interaction
not blocked
not muted where server enforcement applies
rate limit allows
account valid
```

---

## 90. Data Minimization

Store:

```text
what is required for functionality
```

Do not automatically store:

```text
every raw touch event forever
every presence update
every heartbeat
```

Ephemeral events should remain ephemeral unless explicitly needed.

---

## 91. Product-Specific Admin

Use Core admin framework and add Pulse panels for:

```text
Pulse product health
interaction volume
delivery failures
abuse
reported users
Mood configuration
feature flags
remote configuration
Pulse limits
Live Touch health
```

Do not build an entirely separate admin ecosystem.

---

## 92. Feature Flags

Pulse features should be independently flaggable.

Examples:

```text
pulse
pulse_back
knock
mood
live_touch
custom_signals
moments
scheduled_pulse
wearables
```

Use Core Feature Flags.

---

## 93. Remote Configuration

Examples:

```text
pulse.max_duration_ms

pulse.rate_limit

knock.rate_limit

mood.available_visuals

mood.default_expiry

live_touch.max_session_duration

moments.retention_days
```

Do not require mobile release for simple operational changes.

---

## 94. Experimentation

Future A/B tests may include:

```text
Mood visual styles
Pulse animation
invite wording
onboarding order
home navigation
Pulse button style
```

Use Core experimentation framework.

Do not hardwire experimental logic into UI.

---

## 95. Observability

Every Pulse interaction should have:

```text
interactionId
traceId
correlationId
```

Example debugging:

```text
Pulse did not arrive

interaction:
01...

sender gateway:
success

relationship authorization:
success

receiver presence:
offline

notification requested:
success

FCM:
failed

reason:
expired device token
```

This should be diagnosable without manually correlating unrelated logs.

---

## 96. Realtime Metrics

Measure:

```text
active WebSocket connections
connection failures
reconnections
Pulse routing latency
Live Touch latency
cross-region latency
delivery success
push fallback percentage
```

---

## 97. SLO Candidates

Eventually define product SLOs such as:

```text
API availability
Realtime availability
Pulse routing latency
Push request success
Live Touch session setup success
```

Do not claim unrealistic zero-latency behaviour.

---

## 98. Product Architecture Visibility

Every Pulse module must be registered in the existing developer/control-plane catalog.

For each module expose:

```text
owner
purpose
repository location
APIs
events
dependencies
data ownership
dashboards
alerts
runbooks
deployment
```

AI and humans should clearly understand how Pulse fits into Core.

---

## 99. AI Development Rules

Before modifying Pulse:

1. Read Core `AI_CONTEXT.md`.
2. Read Core architecture.
3. Read Pulse product architecture.
4. Inspect existing Core capabilities.
5. Never rebuild an existing Core capability.
6. Never modify Core merely to make a Pulse shortcut unless the change is truly generic.
7. If Core must change, explain why the capability benefits all future apps.
8. Keep Pulse-specific changes in Pulse.
9. Update contracts.
10. Add tests.
11. Validate actual behaviour.
12. Update documentation.
13. Review `git diff`.
14. Commit focused changes.

---

## 100. Repository Integration

Do not assume a new repository is required.

Inspect the existing Core monorepo.

Recommended structure may resemble:

```text
apps/
└── pulse/
    ├── mobile/
    ├── api/
    ├── modules/
    │   ├── profile/
    │   ├── bond/
    │   ├── mood/
    │   ├── interaction/
    │   ├── signals/
    │   ├── live-touch/
    │   └── moments/
    ├── contracts/
    ├── docs/
    └── tests/
```

Follow existing repository conventions if different.

---

## 101. Implementation Strategy

Do NOT attempt to build the entire application in one AI change.

Implement vertical slices.

Each slice should result in working behaviour.

---

## PHASE 1 — PRODUCT FOUNDATION

Implement:

```text
Pulse application registration
Pulse module boundaries
Pulse-specific config
Pulse profile extension
mobile navigation shell
Core SDK integration
feature flag namespace
analytics namespace
Pulse documentation
```

Validate everything.

---

## PHASE 2 — CONNECTION EXPERIENCE

Use Core relationships.

Implement Pulse UI for:

```text
find/invite
request
accept
decline
remove
friend
close friend
```

Do not implement Bond yet unless base connections are validated.

---

## PHASE 3 — PARTNER BOND

Implement:

```text
Bond request
Bond acceptance
one-active-Bond policy
Bond ending
Bond-specific permissions
```

Tests must cover concurrency.

Two simultaneous Bond requests must not violate the one-active-Bond rule.

---

## PHASE 4 — BASIC PULSE

Implement the primary feature:

```text
select connected user
hold button
PulseStart
PulseStop
server duration
receiver realtime delivery
sender feedback
receiver native haptic
durable completion record
analytics
observability
```

This phase is critical.

Do not proceed until it works reliably between two devices/simulators where technically possible.

---

## PHASE 5 — PUSH FALLBACK

Implement:

```text
receiver offline
      ↓
Core Notification
      ↓
push
```

Validate iOS and Android separately.

Document OS limitations.

---

## PHASE 6 — PULSE BACK

Implement rapid reciprocal interaction.

Measure:

```text
Pulse received
→ Pulse Back
```

---

## PHASE 7 — KNOCK

Implement short predefined haptic pattern.

Then make architecture extensible toward custom signals.

---

## PHASE 8 — TODAY'S MOOD

Implement:

```text
Mood selection
Mood audience
Mood expiry
Mood display
Mood response with Pulse/Knock
```

Keep it wordless-first.

---

## PHASE 9 — CLOSE FRIENDS & CIRCLES

Integrate Core groups/relationship capabilities.

Implement Mood audience and interaction permissions.

---

## PHASE 10 — LIVE TOUCH

Implement:

```text
session invitation
session acceptance
realtime presence
touch start
touch stop
haptic response
disconnect behaviour
session completion
latency telemetry
```

This is a flagship feature.

Treat network/haptic quality as highly important.

---

## PHASE 11 — CUSTOM TOUCH LANGUAGE

Implement signal pattern creation.

Support safe bounded patterns.

Do not allow malicious infinite/very long vibration patterns.

---

## PHASE 12 — MOMENTS

Implement:

```text
recent interactions
save moment
retention
privacy
```

No chat.

---

## PHASE 13 — QUIET HOURS / EXPERIENCE CONTROLS

Integrate:

```text
quiet hours
mute
notification privacy
haptic intensity where possible
device preferences
```

---

## PHASE 14 — TRUST & SAFETY

Validate:

```text
block
report
abuse rate limiting
spam prevention
Bond ending
relationship removal
```

---

## PHASE 15 — POLISH

Implement:

```text
animations
accessibility
localization
onboarding
deep links
invite flow
offline UX
error UX
performance
battery optimization
```

---

## PHASE 16 — MONETISATION FOUNDATION

Integrate Core Entitlements.

Do not block core Pulse communication.

Prepare premium capabilities behind flags.

---

## PHASE 17 — WEARABLE PREPARATION

Only after mobile experience is stable.

Design architecture for:

```text
Apple Watch
Wear OS
```

Do not compromise mobile architecture for premature wearable implementation.

---

## 102. Definition of Done for Basic Pulse

Basic Pulse is only complete when:

```text
two users can connect

sender can select receiver

sender can press and hold

local visual responds

PulseStart reaches server

authorization is checked

receiver receives Live Pulse when foreground/online

receiver haptic starts

sender release produces PulseStop

receiver haptic stops

duration is server-calculated/validated

interaction is persisted appropriately

receiver offline triggers push fallback

duplicates are prevented

blocking prevents Pulse

rate limits work

analytics event exists

trace exists

logs are structured

tests pass

iOS behaviour documented

Android behaviour documented
```

---

## 103. Definition of Done for Today's Mood

Mood is complete when:

```text
user selects visual Mood

user selects audience

Mood persists

Mood expires correctly

timezone works

unauthorized users cannot view Mood

authorized connections see Mood

Mood can be updated

Mood can be cleared

Mood can receive non-verbal response

analytics exists

audit where required

tests pass
```

---

## 104. Definition of Done for Live Touch

Live Touch is complete when:

```text
authorized users can create session

receiver can accept

both realtime connections established

touch start routes correctly

touch stop routes correctly

remote haptic responds

disconnect handled

timeout handled

block/mute enforced

rate limiting enforced

latency measured

session completes cleanly

mobile battery impact assessed

iOS limitations documented

Android limitations documented

tests and runtime validation completed
```

---

## 105. Product Principles That Must Never Be Lost

Keep these principles visible during all development:

```text
Pulse is non-verbal first.

Touch is more important than text.

Emotion is more important than engagement metrics.

The product should feel private.

Partner is special, but Pulse is not couples-only.

A user can have one Partner and multiple Friends.

Today's Mood is passive emotional presence.

Pulse is active emotional presence.

Live Touch is synchronous presence.

Knock is lightweight attention.

Custom Signals become a private language.

Do not create surveillance.

Do not create relationship pressure.

Do not create manipulative streaks.

Do not become another messaging app.

Core Platform owns generic infrastructure.

Pulse owns Pulse-specific product behaviour.

Every interaction must respect consent, permissions, blocking and privacy.

The application must remain usable globally.

iOS and Android platform limitations must be handled honestly.

Realtime behaviour must degrade gracefully to notifications.

Everything important must be observable.

Every AI change must remain understandable and reviewable.
```

---

## 106. FIRST IMPLEMENTATION TASK

Do not immediately code the complete application.

Perform this first:

### Pulse Integration & Architecture Audit

1. Inspect the complete existing Core Platform.
2. Read Core `AI_CONTEXT.md`.
3. Identify every Core capability Pulse can reuse.
4. Identify any genuine missing generic Core capabilities.
5. Do NOT modify Core yet.
6. Produce a Core-to-Pulse capability map.
7. Define Pulse module boundaries.
8. Define Pulse-owned data.
9. Define Core-owned data.
10. Define REST contracts required.
11. Define realtime events required.
12. Define durable events required.
13. Define authorization rules.
14. Define feature flags.
15. Define remote configuration.
16. Define analytics events.
17. Define observability requirements.
18. Define iOS native haptic boundary.
19. Define Android native haptic boundary.
20. Define mobile navigation.
21. Define Phase 1 repository changes.
22. Create/update Pulse architecture documentation.
23. Validate repository state.
24. Show the proposed implementation plan.

Then STOP before large-scale implementation.

Report:

```text
CORE CAPABILITIES REUSED

PULSE-SPECIFIC CAPABILITIES

CORE GAPS

DATA OWNERSHIP

MODULE BOUNDARIES

API CONTRACTS

REALTIME CONTRACTS

EVENT CONTRACTS

AUTHORIZATION MODEL

MOBILE ARCHITECTURE

RISKS

PHASED IMPLEMENTATION PLAN

FIRST CODE CHANGE RECOMMENDED
```

Do not start implementing the complete product until this architecture audit is complete and internally consistent.

---

## Final Goal

The finished product should allow a user to:

```text
Create an account
      ↓
Connect with people
      ↓
Have one special Partner Bond
      +
Friends / Close Friends / Circles
      ↓
Set Today's Mood
      ↓
See permitted Moods from people they care about
      ↓
Send a Pulse
      ↓
Feel a Pulse
      ↓
Pulse Back
      ↓
Knock
      ↓
Create a private touch language
      ↓
Enter Live Touch
      ↓
Share meaningful non-verbal moments
```

The experience should eventually reach the point where:

> **Two people can communicate meaningfully without typing a single word.**

That is the defining vision of Pulse.
