import 'dart:async';

import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:core_sdk/core_sdk.dart';

import 'haptic_engine.dart';
import 'pulse_api.dart';

const _keycloakUrl = String.fromEnvironment('KEYCLOAK_URL', defaultValue: 'http://localhost:8081');
const _coreApiUrl = String.fromEnvironment('CORE_API_URL', defaultValue: 'http://localhost:8080');
const _pulseApiUrl = String.fromEnvironment('PULSE_API_URL', defaultValue: 'http://localhost:8096');
const _realtimeUrl = String.fromEnvironment('REALTIME_URL', defaultValue: 'http://localhost:8090');
const _realm = String.fromEnvironment('KEYCLOAK_REALM', defaultValue: 'core');
const _clientId = String.fromEnvironment('KEYCLOAK_CLIENT_ID', defaultValue: 'core-platform');

void main() => runApp(const PulseApp());

class PulseApp extends StatelessWidget {
  const PulseApp({super.key});
  @override
  Widget build(BuildContext context) => MaterialApp(
        title: 'Pulse',
        theme: ThemeData(
          useMaterial3: true,
          colorSchemeSeed: const Color(0xFFE11D5E),
          brightness: Brightness.dark,
        ),
        home: const LoginPage(),
      );
}

/// Pulse never implements separate authentication (product spec §53) -
/// this is the same real Keycloak password-grant + core_sdk pattern
/// apps/mobile's own LoginPage established, reused verbatim rather than
/// reinvented.
class LoginPage extends StatefulWidget {
  final String keycloakUrl;
  final String coreApiUrl;
  final String pulseApiUrl;
  final String realtimeUrl;
  final String realm;
  final String clientId;
  final http.Client? httpClient;

  const LoginPage({
    super.key,
    this.keycloakUrl = _keycloakUrl,
    this.coreApiUrl = _coreApiUrl,
    this.pulseApiUrl = _pulseApiUrl,
    this.realtimeUrl = _realtimeUrl,
    this.realm = _realm,
    this.clientId = _clientId,
    this.httpClient,
  });

  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> {
  final _username = TextEditingController(text: 'demo');
  final _password = TextEditingController(text: 'demo');
  String? _error;
  bool _loading = false;

  Future<void> _login() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final tokens = PasswordTokenSource(
      keycloakUrl: widget.keycloakUrl,
      realm: widget.realm,
      clientId: widget.clientId,
      username: _username.text,
      password: _password.text,
      httpClient: widget.httpClient,
    );
    final coreApi = CoreApi(widget.coreApiUrl, tokenSource: tokens, httpClient: widget.httpClient);
    final pulseApi = PulseApi(widget.pulseApiUrl, tokenSource: tokens, httpClient: widget.httpClient);
    try {
      // Fails fast with a real auth error before navigating anywhere -
      // same discipline apps/mobile's LoginPage established.
      await coreApi.identityMe();
      if (!mounted) return;
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(
          builder: (_) => HomeShell(coreApi: coreApi, pulseApi: pulseApi, tokens: tokens, realtimeUrl: widget.realtimeUrl),
        ),
      );
    } catch (err) {
      setState(() => _error = '$err');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
        body: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 360),
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text('Pulse', style: Theme.of(context).textTheme.displaySmall?.copyWith(fontWeight: FontWeight.w800)),
                  const SizedBox(height: 4),
                  Text('Feel it instead of reading it.', style: Theme.of(context).textTheme.bodyMedium?.copyWith(fontStyle: FontStyle.italic)),
                  const SizedBox(height: 28),
                  TextField(controller: _username, decoration: const InputDecoration(labelText: 'Username')),
                  const SizedBox(height: 8),
                  TextField(controller: _password, decoration: const InputDecoration(labelText: 'Password'), obscureText: true),
                  const SizedBox(height: 16),
                  if (_error != null) Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                  const SizedBox(height: 8),
                  FilledButton(onPressed: _loading ? null : _login, child: Text(_loading ? 'Signing in…' : 'Sign in')),
                ],
              ),
            ),
          ),
        ),
      );
}

/// The five-tab navigation shell from product spec §44 - Home, People,
/// Mood, Moments, Profile. Owns the one persistent realtime connection
/// (product spec §61's Live delivery path) so incoming pulse.started/
/// pulse.stopped pushes trigger real haptic feedback regardless of
/// which tab is showing - a Pulse should be felt even if the receiver
/// isn't looking at Home.
class HomeShell extends StatefulWidget {
  final CoreApi coreApi;
  final PulseApi pulseApi;
  final TokenSource tokens;
  final String realtimeUrl;
  const HomeShell({super.key, required this.coreApi, required this.pulseApi, required this.tokens, required this.realtimeUrl});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _tab = 0;
  final HapticEngine _haptics = HapticEngine.detect();
  RealtimeConn? _conn;
  StreamSubscription<RealtimeMessage>? _sub;
  String? _incomingBanner;
  String? _incomingInteractionId;

  // Live Touch (product spec §21, Phase 10) - gated on a real active
  // Bond, so the Home tab only offers it against the real bond partner.
  String? _bondPartnerId;
  // An incoming invite (someone else invited me) - Accept/Decline shown
  // via _IncomingLiveTouchBanner, kept separate from the Pulse banner
  // above since the two carry different actions.
  String? _liveTouchInviteSessionId;
  // My own outgoing invite, waiting on the other side to respond.
  String? _liveTouchWaitingSessionId;
  String? _liveTouchStatusMessage;

  @override
  void initState() {
    super.initState();
    _connectRealtime();
    _loadBondPartner();
  }

  Future<void> _loadBondPartner() async {
    try {
      final bond = await widget.pulseApi.myBond();
      if (mounted) setState(() => _bondPartnerId = bond.otherUserId);
    } catch (_) {
      // No active Bond - Live Touch simply stays unavailable, not an error.
    }
  }

  Future<void> _connectRealtime() async {
    try {
      // A real registered device, exactly like apps/mobile's own
      // ProfilePage - realtime-gateway's dial contract requires one.
      final device = await widget.coreApi.devicesRegister(
        RegisterDeviceInput(clientDeviceId: 'pulse-mobile', platform: 'flutter'),
      );
      final conn = await RealtimeClient(widget.realtimeUrl, widget.tokens).dial(device.id);
      if (!mounted) {
        await conn.close();
        return;
      }
      setState(() => _conn = conn);
      _sub = conn.messages.listen(_onRealtimeMessage);
    } catch (_) {
      // Live delivery is a bonus, not a requirement - the app still
      // works (via HTTP + the eventual Phase 5 push fallback) if the
      // realtime dial fails.
    }
  }

  void _onRealtimeMessage(RealtimeMessage m) {
    switch (m.type) {
      case 'pulse.started':
        _haptics.playPulseStart();
        final data = m.data;
        final interactionId = data is Map ? data['interactionId'] as String? : null;
        setState(() {
          _incomingBanner = 'Someone is pulsing you 💗';
          _incomingInteractionId = interactionId;
        });
        break;
      case 'pulse.stopped':
        _haptics.playPulseStop();
        // Keep the banner (and Pulse Back option) up briefly after the
        // sender releases - product spec §17's whole point is that
        // responding stays fast and available right after feeling it,
        // not gone the instant the gesture itself ends.
        Future.delayed(const Duration(seconds: 8), () {
          if (mounted) setState(() => _incomingBanner = null);
        });
        break;
      // Knock (Phase 7, spec §18) is "quicker, lighter... a nudge, not a
      // hold" - felt as its own haptic pattern, with no Pulse-Back-style
      // response action, so the banner never sets _incomingInteractionId
      // and clears itself faster than a held Pulse's does.
      case 'knock.started':
        _haptics.playKnock();
        setState(() {
          _incomingBanner = 'Someone knocked 👋';
          _incomingInteractionId = null;
        });
        break;
      case 'knock.stopped':
        Future.delayed(const Duration(seconds: 3), () {
          if (mounted) setState(() => _incomingBanner = null);
        });
        break;
      // Live Touch session lifecycle (spec §21, Phase 10) - the touch
      // events themselves never arrive here; they flow as plain
      // 'message' events on the session's own channel, handled directly
      // by LiveTouchScreen once both sides are subscribed.
      case 'live_touch.invited':
        _haptics.playKnock();
        final data = m.data;
        final sessionId = data is Map ? data['sessionId'] as String? : null;
        setState(() => _liveTouchInviteSessionId = sessionId);
        break;
      case 'live_touch.accepted':
        final data = m.data;
        final sessionId = data is Map ? data['sessionId'] as String? : null;
        final channel = data is Map ? data['channel'] as String? : null;
        if (sessionId != null && sessionId == _liveTouchWaitingSessionId && channel != null && _conn != null) {
          setState(() => _liveTouchWaitingSessionId = null);
          _openLiveTouchScreen(sessionId, channel);
        }
        break;
      case 'live_touch.declined':
        final data = m.data;
        final sessionId = data is Map ? data['sessionId'] as String? : null;
        if (sessionId != null && sessionId == _liveTouchWaitingSessionId) {
          setState(() {
            _liveTouchWaitingSessionId = null;
            _liveTouchStatusMessage = 'Live Touch declined';
          });
          Future.delayed(const Duration(seconds: 3), () {
            if (mounted) setState(() => _liveTouchStatusMessage = null);
          });
        }
        break;
      // Custom Signals (spec §19-20, Phase 11): "the pattern itself is
      // the communication" - the server never assigns meaning, and
      // neither does this handler; it just plays back whatever segments
      // arrived, exactly once, with no response action (unlike Pulse's
      // own banner) since a private pattern's whole point is felt, not
      // replied to with another gesture.
      case 'signal.started':
        final data = m.data;
        final segments = data is Map ? data['segments'] as List<dynamic>? : null;
        if (segments != null) _haptics.playSegments(segments);
        setState(() => _incomingBanner = 'Someone sent you a Signal 💌');
        Future.delayed(const Duration(seconds: 4), () {
          if (mounted) setState(() => _incomingBanner = null);
        });
        break;
    }
  }

  void _openLiveTouchScreen(String sessionId, String channel) {
    final conn = _conn;
    if (conn == null) return;
    Navigator.of(context).push(MaterialPageRoute(
      builder: (_) => LiveTouchScreen(pulseApi: widget.pulseApi, haptics: _haptics, conn: conn, sessionId: sessionId, channel: channel),
    ));
  }

  Future<void> _inviteLiveTouch() async {
    try {
      final session = await widget.pulseApi.inviteLiveTouch();
      setState(() => _liveTouchWaitingSessionId = session.id);
    } catch (err) {
      setState(() => _liveTouchStatusMessage = 'Could not start Live Touch: $err');
    }
  }

  Future<void> _acceptLiveTouchInvite() async {
    final sessionId = _liveTouchInviteSessionId;
    if (sessionId == null) return;
    setState(() => _liveTouchInviteSessionId = null);
    try {
      final session = await widget.pulseApi.acceptLiveTouch(sessionId);
      if (session.channel != null) _openLiveTouchScreen(sessionId, session.channel!);
    } catch (err) {
      setState(() => _liveTouchStatusMessage = 'Could not accept Live Touch: $err');
    }
  }

  Future<void> _declineLiveTouchInvite() async {
    final sessionId = _liveTouchInviteSessionId;
    if (sessionId == null) return;
    setState(() => _liveTouchInviteSessionId = null);
    try {
      await widget.pulseApi.declineLiveTouch(sessionId);
    } catch (_) {
      // Best-effort - the invite will lazily time out server-side either way.
    }
  }

  Future<void> _pulseBack() async {
    final id = _incomingInteractionId;
    if (id == null) return;
    setState(() => _incomingBanner = 'Pulsing back…');
    try {
      await widget.pulseApi.pulseBack(id);
      _haptics.playPulseStart();
      if (!mounted) return;
      setState(() {
        _incomingBanner = 'Pulsed back 💗';
        _incomingInteractionId = null;
      });
      Future.delayed(const Duration(seconds: 2), () {
        if (mounted) setState(() => _incomingBanner = null);
      });
    } catch (err) {
      if (mounted) setState(() => _incomingBanner = 'Could not Pulse Back: $err');
    }
  }


  @override
  void dispose() {
    _sub?.cancel();
    _conn?.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final pages = [
      _HomeTab(
        pulseApi: widget.pulseApi,
        haptics: _haptics,
        bondPartnerId: _bondPartnerId,
        liveTouchWaiting: _liveTouchWaitingSessionId != null,
        onInviteLiveTouch: _inviteLiveTouch,
      ),
      _PeopleTab(pulseApi: widget.pulseApi),
      _MoodTab(pulseApi: widget.pulseApi),
      _MomentsTab(pulseApi: widget.pulseApi),
      _ProfileTab(pulseApi: widget.pulseApi, haptics: _haptics),
    ];
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            if (_incomingBanner != null)
              _IncomingPulseBanner(
                text: _incomingBanner!,
                onPulseBack: _incomingInteractionId != null ? _pulseBack : null,
              ),
            if (_liveTouchInviteSessionId != null)
              _IncomingLiveTouchBanner(onAccept: _acceptLiveTouchInvite, onDecline: _declineLiveTouchInvite),
            if (_liveTouchStatusMessage != null)
              Container(
                width: double.infinity,
                color: Theme.of(context).colorScheme.secondaryContainer,
                padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 12),
                child: Text(_liveTouchStatusMessage!, textAlign: TextAlign.center),
              ),
            Expanded(child: pages[_tab]),
          ],
        ),
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _tab,
        onDestinationSelected: (i) => setState(() => _tab = i),
        destinations: const [
          NavigationDestination(icon: Icon(Icons.favorite_border), selectedIcon: Icon(Icons.favorite), label: 'Home'),
          NavigationDestination(icon: Icon(Icons.people_outline), selectedIcon: Icon(Icons.people), label: 'People'),
          NavigationDestination(icon: Icon(Icons.wb_sunny_outlined), selectedIcon: Icon(Icons.wb_sunny), label: 'Mood'),
          NavigationDestination(icon: Icon(Icons.auto_awesome_outlined), selectedIcon: Icon(Icons.auto_awesome), label: 'Moments'),
          NavigationDestination(icon: Icon(Icons.person_outline), selectedIcon: Icon(Icons.person), label: 'Profile'),
        ],
      ),
    );
  }
}

/// The incoming-Pulse banner, including the Pulse Back button (product
/// spec §17) - a plain, isolated widget deliberately separate from
/// HomeShell's own realtime-connection state, so it's testable without
/// a real WebSocket connection (this test sandbox can't dial one - see
/// _HomeShellState._connectRealtime's own doc comment).
class _IncomingPulseBanner extends StatelessWidget {
  final String text;
  final VoidCallback? onPulseBack;
  const _IncomingPulseBanner({required this.text, this.onPulseBack});

  @override
  Widget build(BuildContext context) => Container(
        width: double.infinity,
        color: Theme.of(context).colorScheme.primary,
        padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 12),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Flexible(
              child: Text(text, textAlign: TextAlign.center, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600)),
            ),
            if (onPulseBack != null) ...[
              const SizedBox(width: 10),
              TextButton(
                onPressed: onPulseBack,
                style: TextButton.styleFrom(backgroundColor: Colors.white.withValues(alpha: 0.2), foregroundColor: Colors.white),
                child: const Text('Pulse Back'),
              ),
            ],
          ],
        ),
      );
}

/// The incoming Live Touch invite banner (spec §21) - Accept/Decline,
/// never a Pulse-Back-style single action, since a session must be
/// mutually agreed before either side can feel anything.
class _IncomingLiveTouchBanner extends StatelessWidget {
  final VoidCallback onAccept;
  final VoidCallback onDecline;
  const _IncomingLiveTouchBanner({required this.onAccept, required this.onDecline});

  @override
  Widget build(BuildContext context) => Container(
        width: double.infinity,
        color: Theme.of(context).colorScheme.tertiary,
        padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 12),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Flexible(
              child: Text('Your partner wants to Live Touch', textAlign: TextAlign.center, style: TextStyle(color: Colors.white, fontWeight: FontWeight.w600)),
            ),
            const SizedBox(width: 10),
            TextButton(
              key: const Key('acceptLiveTouchButton'),
              onPressed: onAccept,
              style: TextButton.styleFrom(backgroundColor: Colors.white.withValues(alpha: 0.2), foregroundColor: Colors.white),
              child: const Text('Accept'),
            ),
            const SizedBox(width: 6),
            TextButton(
              key: const Key('declineLiveTouchButton'),
              onPressed: onDecline,
              style: TextButton.styleFrom(foregroundColor: Colors.white),
              child: const Text('Decline'),
            ),
          ],
        ),
      );
}

/// The "hold to Pulse" main screen (product spec §13, Phase 4). Press
/// down starts a real Pulse (create+start in one call); release stops
/// it - PulseStart/PulseStop, never a continuous stream (spec §15).
/// Duration shown to the sender is local (for feel); the server's own
/// duration (spec §78, never trusted from the client) is what's
/// actually recorded.
class _HomeTab extends StatefulWidget {
  final PulseApi pulseApi;
  final HapticEngine haptics;
  // Live Touch (spec §21) is Bond-gated - bondPartnerId is null when the
  // caller has no active Bond, in which case the button never appears
  // at all, regardless of which connection is selected.
  final String? bondPartnerId;
  final bool liveTouchWaiting;
  final VoidCallback onInviteLiveTouch;
  const _HomeTab({
    required this.pulseApi,
    required this.haptics,
    this.bondPartnerId,
    this.liveTouchWaiting = false,
    required this.onInviteLiveTouch,
  });

  @override
  State<_HomeTab> createState() => _HomeTabState();
}

class _HomeTabState extends State<_HomeTab> {
  List<PulseConnection> _connections = [];
  PulseConnection? _target;
  String? _error;
  bool _loadingConnections = true;
  // The target's own Today's Mood (product spec §27: "a user sees
  // another person's Mood and can respond without words") - null means
  // either no Mood is set or the caller isn't in its audience; the
  // server deliberately makes those indistinguishable, so this widget
  // never tries to tell them apart either.
  String? _targetMoodEmoji;

  bool _holding = false;
  String? _activeInteractionId;
  DateTime? _holdStartedAt;
  Duration _elapsed = Duration.zero;
  Timer? _ticker;
  String? _lastResult;
  // "Save this moment ♥" (spec §31) - only meaningful once the most
  // recent interaction has actually completed, so this and _lastResult
  // are always set together.
  String? _lastInteractionId;
  bool _momentSaved = false;

  @override
  void initState() {
    super.initState();
    _loadConnections();
  }

  Future<void> _loadConnections() async {
    try {
      final all = await widget.pulseApi.listConnections();
      final active = all.where((c) => c.status == 'active').toList();
      setState(() {
        _connections = active;
        _target = active.isNotEmpty ? active.first : null;
        _loadingConnections = false;
      });
      _loadTargetMood();
    } catch (err) {
      setState(() {
        _error = '$err';
        _loadingConnections = false;
      });
    }
  }

  Future<void> _loadTargetMood() async {
    final target = _target;
    setState(() => _targetMoodEmoji = null);
    if (target == null) return;
    try {
      final mood = await widget.pulseApi.viewMood(target.otherUserId);
      if (mounted && _target?.otherUserId == target.otherUserId) {
        setState(() => _targetMoodEmoji = mood.emoji);
      }
    } catch (_) {
      // No visible Mood - not an error worth surfacing here.
    }
  }

  Future<void> _onPressStart() async {
    final target = _target;
    if (target == null || _holding) return;
    setState(() {
      _holding = true;
      _holdStartedAt = DateTime.now();
      _elapsed = Duration.zero;
      _lastResult = null;
    });
    widget.haptics.playPulseStart();
    _ticker = Timer.periodic(const Duration(milliseconds: 50), (_) {
      if (_holdStartedAt == null) return;
      setState(() => _elapsed = DateTime.now().difference(_holdStartedAt!));
    });
    try {
      final interaction = await widget.pulseApi.createAndStart(target.otherUserId);
      _activeInteractionId = interaction.id;
    } catch (err) {
      setState(() {
        _error = '$err';
        _holding = false;
      });
      _ticker?.cancel();
    }
  }

  Future<void> _onPressEnd() async {
    _ticker?.cancel();
    widget.haptics.playPulseStop();
    final id = _activeInteractionId;
    setState(() => _holding = false);
    if (id == null) return;
    _activeInteractionId = null;
    try {
      final stopped = await widget.pulseApi.stop(id);
      setState(() {
        _lastResult = 'Pulse sent — felt for ${stopped.durationMs ?? 0}ms';
        _lastInteractionId = stopped.id;
        _momentSaved = false;
      });
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  // Knock (spec §18): a short predefined pattern, not a held gesture -
  // one call, felt (or fired-and-forgotten) immediately, unlike Pulse's
  // press/release pair above.
  Future<void> _sendKnock() async {
    final target = _target;
    if (target == null || _holding) return;
    widget.haptics.playKnock();
    try {
      final knock = await widget.pulseApi.knock(target.otherUserId);
      setState(() {
        _lastResult = 'Knock sent';
        _lastInteractionId = knock.id;
        _momentSaved = false;
      });
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  Future<void> _saveMoment() async {
    final id = _lastInteractionId;
    if (id == null) return;
    try {
      await widget.pulseApi.saveMoment(id);
      setState(() => _momentSaved = true);
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (_loadingConnections) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_connections.isEmpty) {
      return const _PlaceholderTab(
        title: 'No connections yet',
        subtitle: 'Connect with someone first (Phase 2) before you can send them a Pulse.',
      );
    }
    final target = _target!;
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (_connections.length > 1)
            Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: Wrap(
                spacing: 8,
                children: _connections
                    .map((c) => ChoiceChip(
                          label: Text(c.otherUserId.substring(0, 8)),
                          selected: c.relationshipId == target.relationshipId,
                          onSelected: _holding
                              ? null
                              : (_) {
                                  setState(() => _target = c);
                                  _loadTargetMood();
                                },
                        ))
                    .toList(),
              ),
            ),
          GestureDetector(
            key: const Key('pulseButton'),
            onTapDown: (_) => _onPressStart(),
            onTapUp: (_) => _onPressEnd(),
            onTapCancel: _onPressEnd,
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 150),
              width: _holding ? 200 : 160,
              height: _holding ? 200 : 160,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: Theme.of(context).colorScheme.primary.withValues(alpha: _holding ? 0.35 : 0.15),
                border: Border.all(color: Theme.of(context).colorScheme.primary, width: _holding ? 3 : 2),
              ),
              child: Icon(Icons.favorite, size: _holding ? 80 : 64, color: Theme.of(context).colorScheme.primary),
            ),
          ),
          const SizedBox(height: 16),
          Text(_holding ? 'Holding… ${(_elapsed.inMilliseconds / 1000).toStringAsFixed(1)}s' : 'Hold to Pulse'),
          const SizedBox(height: 4),
          Text(
            _targetMoodEmoji == null
                ? 'to @${target.otherUserId.substring(0, 8)} (${target.classification})'
                : 'to @${target.otherUserId.substring(0, 8)} (${target.classification}) — feeling $_targetMoodEmoji',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 12),
          OutlinedButton(
            key: const Key('knockButton'),
            onPressed: _holding ? null : _sendKnock,
            child: const Text('Knock'),
          ),
          const SizedBox(height: 8),
          OutlinedButton(
            key: const Key('signalsButton'),
            onPressed: _holding
                ? null
                : () => Navigator.of(context).push(MaterialPageRoute(
                      builder: (_) => SignalsScreen(pulseApi: widget.pulseApi, haptics: widget.haptics, targetUserId: target.otherUserId),
                    )),
            child: const Text('Signals'),
          ),
          if (widget.bondPartnerId != null && target.otherUserId == widget.bondPartnerId) ...[
            const SizedBox(height: 8),
            FilledButton.tonal(
              key: const Key('liveTouchButton'),
              onPressed: _holding || widget.liveTouchWaiting ? null : widget.onInviteLiveTouch,
              child: Text(widget.liveTouchWaiting ? 'Waiting for partner…' : 'Live Touch'),
            ),
          ],
          if (_lastResult != null) ...[
            const SizedBox(height: 12),
            Text(_lastResult!, style: TextStyle(color: Theme.of(context).colorScheme.primary)),
            if (_lastInteractionId != null) ...[
              const SizedBox(height: 4),
              TextButton(
                key: const Key('saveMomentButton'),
                onPressed: _momentSaved ? null : _saveMoment,
                child: Text(_momentSaved ? 'Saved ♥' : 'Save this moment ♥'),
              ),
            ],
          ],
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
          ],
        ],
      ),
    );
  }
}

/// Live Touch (product spec §21, Phase 10) - the flagship synchronous
/// two-way touch feature. Touch-start/touch-stop never go through
/// pulse_api.dart's HTTP client at all: both participants publish and
/// receive them directly over the session's own realtime-gateway
/// channel via the already-connected RealtimeConn (see HomeShell's own
/// single persistent connection), the lowest-latency path this platform
/// has - live-validated end to end at sub-millisecond relay latency
/// during this phase's own real-infrastructure testing.
class LiveTouchScreen extends StatefulWidget {
  final PulseApi pulseApi;
  final HapticEngine haptics;
  final RealtimeConn conn;
  final String sessionId;
  final String channel;
  const LiveTouchScreen({
    super.key,
    required this.pulseApi,
    required this.haptics,
    required this.conn,
    required this.sessionId,
    required this.channel,
  });

  @override
  State<LiveTouchScreen> createState() => _LiveTouchScreenState();
}

class _LiveTouchScreenState extends State<LiveTouchScreen> {
  StreamSubscription<RealtimeMessage>? _sub;
  bool _iAmTouching = false;
  bool _partnerTouching = false;
  bool _ended = false;

  @override
  void initState() {
    super.initState();
    widget.conn.subscribe(widget.channel);
    _sub = widget.conn.messages.listen(_onMessage);
  }

  void _onMessage(RealtimeMessage m) {
    if (m.type == 'message' && m.channel == widget.channel) {
      final data = m.data;
      final touchType = data is Map ? data['type'] as String? : null;
      if (touchType == 'touch.start') {
        widget.haptics.playPulseStart();
        if (mounted) setState(() => _partnerTouching = true);
      } else if (touchType == 'touch.stop') {
        widget.haptics.playPulseStop();
        if (mounted) setState(() => _partnerTouching = false);
      }
      return;
    }
    if (m.type == 'live_touch.ended') {
      final data = m.data;
      final sessionId = data is Map ? data['sessionId'] as String? : null;
      if (sessionId == widget.sessionId && mounted) {
        setState(() => _ended = true);
        Future.delayed(const Duration(seconds: 1), () {
          if (mounted) Navigator.of(context).pop();
        });
      }
    }
  }

  void _onTouchStart() {
    setState(() => _iAmTouching = true);
    widget.haptics.playPulseStart();
    widget.conn.publish(widget.channel, {'type': 'touch.start'});
  }

  void _onTouchStop() {
    setState(() => _iAmTouching = false);
    widget.haptics.playPulseStop();
    widget.conn.publish(widget.channel, {'type': 'touch.stop'});
  }

  Future<void> _end() async {
    try {
      await widget.pulseApi.endLiveTouch(widget.sessionId);
    } catch (_) {
      // Best-effort - leaving the screen ends the local experience
      // either way; the session lazily times out server-side if the
      // End call itself failed to land.
    }
    if (mounted) Navigator.of(context).pop();
  }

  @override
  void dispose() {
    _sub?.cancel();
    widget.conn.unsubscribe(widget.channel);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final glowing = _iAmTouching || _partnerTouching;
    return Scaffold(
      appBar: AppBar(title: const Text('Live Touch')),
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (_ended)
              const Text('Your partner ended the session')
            else ...[
              GestureDetector(
                key: const Key('liveTouchArea'),
                onTapDown: (_) => _onTouchStart(),
                onTapUp: (_) => _onTouchStop(),
                onTapCancel: _onTouchStop,
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 150),
                  width: glowing ? 220 : 180,
                  height: glowing ? 220 : 180,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: Theme.of(context).colorScheme.tertiary.withValues(alpha: glowing ? 0.4 : 0.15),
                    border: Border.all(color: Theme.of(context).colorScheme.tertiary, width: glowing ? 4 : 2),
                  ),
                  child: Icon(Icons.favorite, size: glowing ? 96 : 72, color: Theme.of(context).colorScheme.tertiary),
                ),
              ),
              const SizedBox(height: 16),
              Text(_partnerTouching ? 'Your partner is touching' : 'Hold to touch'),
              const SizedBox(height: 24),
              OutlinedButton(key: const Key('endLiveTouchButton'), onPressed: _end, child: const Text('End')),
            ],
          ],
        ),
      ),
    );
  }
}

/// Custom Signals (product spec §19-20, Phase 11): a private touch
/// pattern library bound to exactly one specific connection - "the same
/// pattern means something different to different pairs, so it only
/// ever means one thing." Composed here from tap/hold/pause segments,
/// previewable locally before saving (the server never interprets what
/// a pattern means, spec §20 - only this screen, and only for its own
/// creator, ever "reads" one).
class SignalsScreen extends StatefulWidget {
  final PulseApi pulseApi;
  final HapticEngine haptics;
  final String targetUserId;
  const SignalsScreen({super.key, required this.pulseApi, required this.haptics, required this.targetUserId});

  @override
  State<SignalsScreen> createState() => _SignalsScreenState();
}

class _SignalsScreenState extends State<SignalsScreen> {
  List<PulseSignal> _signals = [];
  bool _loading = true;
  String? _error;
  String? _status;
  String? _lastSentInteractionId;
  bool _momentSaved = false;

  final _labelController = TextEditingController();
  final List<Map<String, dynamic>> _composing = [];

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final all = await widget.pulseApi.listSignals();
      final mine = all.where((s) => s.targetUserId == widget.targetUserId).toList();
      if (mounted) setState(() => _signals = mine);
    } catch (err) {
      if (mounted) setState(() => _error = '$err');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _addSegment(String type, int durationMs) {
    setState(() => _composing.add({'type': type, 'durationMs': durationMs}));
  }

  void _clearComposing() {
    setState(() {
      _composing.clear();
      _labelController.clear();
    });
  }

  Future<void> _preview() => widget.haptics.playSegments(_composing);

  Future<void> _save() async {
    if (_composing.isEmpty) return;
    try {
      final sig = await widget.pulseApi.createSignal(widget.targetUserId, _labelController.text.trim(), List.of(_composing));
      setState(() {
        _signals = [..._signals, sig];
        _status = 'Signal saved';
      });
      _clearComposing();
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  Future<void> _send(PulseSignal sig) async {
    try {
      final sent = await widget.pulseApi.sendSignal(sig.id);
      setState(() {
        _status = 'Sent "${sig.label.isEmpty ? 'Signal' : sig.label}"';
        _lastSentInteractionId = sent.id;
        _momentSaved = false;
      });
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  Future<void> _saveMoment() async {
    final id = _lastSentInteractionId;
    if (id == null) return;
    try {
      await widget.pulseApi.saveMoment(id);
      setState(() => _momentSaved = true);
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  Future<void> _delete(PulseSignal sig) async {
    try {
      await widget.pulseApi.deleteSignal(sig.id);
      setState(() => _signals = _signals.where((s) => s.id != sig.id).toList());
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  @override
  void dispose() {
    _labelController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return Scaffold(appBar: AppBar(title: const Text('Signals')), body: const Center(child: CircularProgressIndicator()));
    }
    return Scaffold(
      appBar: AppBar(title: const Text('Signals')),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Text('A private touch pattern only the two of you understand.', style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 16),
          if (_signals.isEmpty) const Text('No signals yet for this connection.'),
          for (final sig in _signals)
            Card(
              child: ListTile(
                title: Text(sig.label.isEmpty ? '(no label)' : sig.label),
                subtitle: Text('${sig.segments.length} segments'),
                trailing: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    IconButton(key: const Key('sendSignalButton'), icon: const Icon(Icons.send), onPressed: () => _send(sig)),
                    IconButton(icon: const Icon(Icons.delete_outline), onPressed: () => _delete(sig)),
                  ],
                ),
              ),
            ),
          const Divider(height: 32),
          Text('Compose a new pattern', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 8),
          TextField(controller: _labelController, decoration: const InputDecoration(labelText: 'Label (only you see this)')),
          const SizedBox(height: 12),
          Wrap(
            spacing: 8,
            children: [
              OutlinedButton(key: const Key('addTapButton'), onPressed: () => _addSegment('tap', 150), child: const Text('+ Tap')),
              OutlinedButton(key: const Key('addHoldButton'), onPressed: () => _addSegment('hold', 900), child: const Text('+ Hold')),
              OutlinedButton(key: const Key('addPauseButton'), onPressed: () => _addSegment('pause', 300), child: const Text('+ Pause')),
            ],
          ),
          const SizedBox(height: 8),
          Text(_composing.isEmpty ? 'No segments yet' : _composing.map((s) => s['type']).join(' → ')),
          const SizedBox(height: 12),
          Row(
            children: [
              OutlinedButton(onPressed: _composing.isEmpty ? null : _preview, child: const Text('Preview')),
              const SizedBox(width: 8),
              FilledButton(key: const Key('saveSignalButton'), onPressed: _composing.isEmpty ? null : _save, child: const Text('Save')),
              const SizedBox(width: 8),
              TextButton(onPressed: _composing.isEmpty ? null : _clearComposing, child: const Text('Clear')),
            ],
          ),
          if (_status != null) ...[
            const SizedBox(height: 12),
            Text(_status!, style: TextStyle(color: Theme.of(context).colorScheme.primary)),
            if (_lastSentInteractionId != null)
              TextButton(
                onPressed: _momentSaved ? null : _saveMoment,
                child: Text(_momentSaved ? 'Saved ♥' : 'Save this moment ♥'),
              ),
          ],
          if (_error != null) ...[
            const SizedBox(height: 12),
            Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
          ],
        ],
      ),
    );
  }
}

/// Today's Mood (product spec §22-27, Phase 8): a single visual symbol,
/// not a status sentence (spec §23's own design philosophy) - the
/// server resolves the chosen audience into who can actually see it, so
/// this widget only ever sends the audience label, never a computed
/// viewer list.
class _MoodTab extends StatefulWidget {
  final PulseApi pulseApi;
  const _MoodTab({required this.pulseApi});

  @override
  State<_MoodTab> createState() => _MoodTabState();
}

class _MoodTabState extends State<_MoodTab> {
  static const _emojis = ['☀️', '🌧️', '🌙', '🔥', '🌊', '🫂', '❤️', '💤'];
  static const _audiences = ['private', 'partner_only', 'close_friends', 'all_connections', 'selected_circles'];

  String? _selectedEmoji;
  String _selectedAudience = 'all_connections';
  List<PulseCircle> _circles = [];
  String? _selectedCircleId;
  PulseMood? _current;
  bool _loading = true;
  String? _error;
  String? _status;

  @override
  void initState() {
    super.initState();
    _loadCurrent();
    _loadCircles();
  }

  Future<void> _loadCurrent() async {
    try {
      final m = await widget.pulseApi.myMood();
      if (mounted) setState(() => _current = m);
    } catch (_) {
      // No Mood currently set - same as a 404, not an error worth
      // surfacing on first load.
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _loadCircles() async {
    try {
      final circles = await widget.pulseApi.listCircles();
      if (mounted) setState(() => _circles = circles);
    } catch (_) {
      // Circles are optional for Mood - an empty list just means the
      // selected_circles option has nothing to pick from yet.
    }
  }

  Future<void> _set() async {
    final emoji = _selectedEmoji;
    if (emoji == null) return;
    if (_selectedAudience == 'selected_circles' && _selectedCircleId == null) {
      setState(() => _error = 'Pick a Circle first');
      return;
    }
    setState(() {
      _status = null;
      _error = null;
    });
    try {
      final m = await widget.pulseApi.setMood(
        emoji,
        _selectedAudience,
        circleId: _selectedAudience == 'selected_circles' ? _selectedCircleId : null,
      );
      setState(() {
        _current = m;
        _status = 'Mood set';
      });
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  Future<void> _clear() async {
    try {
      await widget.pulseApi.clearMood();
      setState(() {
        _current = null;
        _status = 'Mood cleared';
      });
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    return SingleChildScrollView(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text("Today's Mood", style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 4),
          Text('How am I feeling today? No words needed.', style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 16),
          if (_current != null) ...[
            Center(child: Text(_current!.emoji, style: const TextStyle(fontSize: 48))),
            const SizedBox(height: 4),
            Center(child: Text('Visible until ${_current!.expiresAt}', style: Theme.of(context).textTheme.bodySmall)),
            const SizedBox(height: 8),
            Center(
              child: OutlinedButton(key: const Key('clearMoodButton'), onPressed: _clear, child: const Text('Clear Mood')),
            ),
            const SizedBox(height: 16),
            const Divider(),
            const SizedBox(height: 8),
          ],
          Wrap(
            spacing: 8,
            runSpacing: 8,
            alignment: WrapAlignment.center,
            children: _emojis
                .map((e) => ChoiceChip(
                      label: Text(e, style: const TextStyle(fontSize: 20)),
                      selected: _selectedEmoji == e,
                      onSelected: (_) => setState(() => _selectedEmoji = e),
                    ))
                .toList(),
          ),
          const SizedBox(height: 16),
          Center(
            child: DropdownButton<String>(
              key: const Key('moodAudienceDropdown'),
              value: _selectedAudience,
              items: _audiences.map((a) => DropdownMenuItem(value: a, child: Text(a.replaceAll('_', ' ')))).toList(),
              onChanged: (v) => setState(() => _selectedAudience = v ?? _selectedAudience),
            ),
          ),
          if (_selectedAudience == 'selected_circles') ...[
            const SizedBox(height: 8),
            Center(
              child: _circles.isEmpty
                  ? Text('No Circles yet — create one on the People tab.', style: Theme.of(context).textTheme.bodySmall)
                  : DropdownButton<String>(
                      key: const Key('moodCircleDropdown'),
                      hint: const Text('Pick a Circle'),
                      value: _selectedCircleId,
                      items: _circles.map((c) => DropdownMenuItem(value: c.id, child: Text(c.name))).toList(),
                      onChanged: (v) => setState(() => _selectedCircleId = v),
                    ),
            ),
          ],
          const SizedBox(height: 16),
          FilledButton(
            key: const Key('setMoodButton'),
            onPressed: _selectedEmoji == null ? null : _set,
            child: const Text('Set Mood'),
          ),
          if (_status != null) ...[
            const SizedBox(height: 12),
            Center(child: Text(_status!, style: TextStyle(color: Theme.of(context).colorScheme.primary))),
          ],
          if (_error != null) ...[
            const SizedBox(height: 12),
            Center(child: Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error))),
          ],
        ],
      ),
    );
  }
}

/// Circles (product spec §10, Phase 9): custom groups - "Closest
/// Friends, Family, University, Work Friends, Gaming Friends, Custom" -
/// that primarily control Mood audience. A thin UI over Pulse's own
/// pulse-connections wrapper around Core's real groups capability;
/// membership is restricted server-side to real, active connections
/// (never a stranger), so this widget only ever offers existing
/// connections in the "add member" picker.
class _PeopleTab extends StatefulWidget {
  final PulseApi pulseApi;
  const _PeopleTab({required this.pulseApi});

  @override
  State<_PeopleTab> createState() => _PeopleTabState();
}

class _PeopleTabState extends State<_PeopleTab> {
  List<PulseCircle> _circles = [];
  List<PulseConnection> _connections = [];
  bool _loading = true;
  String? _error;
  final _newCircleName = TextEditingController();

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final circles = await widget.pulseApi.listCircles();
      final connections = await widget.pulseApi.listConnections();
      setState(() {
        _circles = circles;
        _connections = connections.where((c) => c.status == 'active').toList();
        _loading = false;
      });
    } catch (err) {
      setState(() {
        _error = '$err';
        _loading = false;
      });
    }
  }

  Future<void> _createCircle() async {
    final name = _newCircleName.text.trim();
    if (name.isEmpty) return;
    try {
      final circle = await widget.pulseApi.createCircle(name);
      setState(() => _circles = [..._circles, circle]);
      _newCircleName.clear();
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text('Circles', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 4),
        Text(
          'Custom groups — Family, Work Friends, Closest Friends — that control Mood audience.',
          style: Theme.of(context).textTheme.bodySmall,
        ),
        const SizedBox(height: 16),
        Row(
          children: [
            Expanded(
              child: TextField(
                key: const Key('newCircleNameField'),
                controller: _newCircleName,
                decoration: const InputDecoration(labelText: 'New Circle name'),
              ),
            ),
            const SizedBox(width: 8),
            FilledButton(key: const Key('createCircleButton'), onPressed: _createCircle, child: const Text('Create')),
          ],
        ),
        const SizedBox(height: 16),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
          ),
        if (_circles.isEmpty) const Text('No Circles yet.'),
        for (final circle in _circles) _CircleTile(key: ValueKey(circle.id), pulseApi: widget.pulseApi, circle: circle, connections: _connections),
      ],
    );
  }
}

class _CircleTile extends StatefulWidget {
  final PulseApi pulseApi;
  final PulseCircle circle;
  final List<PulseConnection> connections;
  const _CircleTile({super.key, required this.pulseApi, required this.circle, required this.connections});

  @override
  State<_CircleTile> createState() => _CircleTileState();
}

class _CircleTileState extends State<_CircleTile> {
  List<PulseCircleMember>? _members;
  String? _selectedToAdd;
  String? _error;

  Future<void> _loadMembers() async {
    try {
      final members = await widget.pulseApi.listCircleMembers(widget.circle.id);
      if (mounted) setState(() => _members = members);
    } catch (err) {
      if (mounted) setState(() => _error = '$err');
    }
  }

  Future<void> _addMember() async {
    final userId = _selectedToAdd;
    if (userId == null) return;
    try {
      await widget.pulseApi.addCircleMember(widget.circle.id, userId);
      await _loadMembers();
      setState(() => _selectedToAdd = null);
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  Future<void> _removeMember(String userId) async {
    try {
      await widget.pulseApi.removeCircleMember(widget.circle.id, userId);
      await _loadMembers();
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  @override
  Widget build(BuildContext context) {
    final members = _members;
    final memberIds = members?.map((m) => m.userId).toSet() ?? const <String>{};
    final addable = widget.connections.where((c) => !memberIds.contains(c.otherUserId)).toList();
    return Card(
      child: ExpansionTile(
        key: PageStorageKey(widget.circle.id),
        title: Text(widget.circle.name),
        onExpansionChanged: (expanded) {
          if (expanded && members == null) _loadMembers();
        },
        children: [
          if (members == null)
            const Padding(padding: EdgeInsets.all(16), child: CircularProgressIndicator())
          else ...[
            for (final m in members)
              ListTile(
                dense: true,
                title: Text(m.userId.substring(0, 8)),
                trailing: IconButton(icon: const Icon(Icons.close), onPressed: () => _removeMember(m.userId)),
              ),
            if (addable.isNotEmpty)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                child: Row(
                  children: [
                    Expanded(
                      child: DropdownButton<String>(
                        key: const Key('addCircleMemberDropdown'),
                        hint: const Text('Add a connection'),
                        value: _selectedToAdd,
                        isExpanded: true,
                        items: addable.map((c) => DropdownMenuItem(value: c.otherUserId, child: Text(c.otherUserId.substring(0, 8)))).toList(),
                        onChanged: (v) => setState(() => _selectedToAdd = v),
                      ),
                    ),
                    IconButton(
                      key: const Key('addCircleMemberButton'),
                      icon: const Icon(Icons.add),
                      onPressed: _selectedToAdd == null ? null : _addMember,
                    ),
                  ],
                ),
              ),
          ],
          if (_error != null)
            Padding(padding: const EdgeInsets.all(16), child: Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error))),
          const SizedBox(height: 8),
        ],
      ),
    );
  }
}

/// Moments (product spec §30-31, Phase 12): a private saved-highlights
/// timeline - "avoid turning this into chat history." Any stats shown
/// are deliberately gentle (spec §32: "12 little moments together this
/// week," never a punishing streak count) and computed locally from the
/// already-fetched list, no dedicated backend endpoint for it.
class _MomentsTab extends StatefulWidget {
  final PulseApi pulseApi;
  const _MomentsTab({required this.pulseApi});

  @override
  State<_MomentsTab> createState() => _MomentsTabState();
}

class _MomentsTabState extends State<_MomentsTab> {
  List<PulseMoment> _moments = [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final list = await widget.pulseApi.listMoments();
      if (mounted) setState(() => _moments = list);
    } catch (err) {
      if (mounted) setState(() => _error = '$err');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _delete(PulseMoment m) async {
    try {
      await widget.pulseApi.deleteMoment(m.id);
      setState(() => _moments = _moments.where((x) => x.id != m.id).toList());
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  int get _thisWeekCount {
    final weekAgo = DateTime.now().toUtc().subtract(const Duration(days: 7));
    return _moments.where((m) {
      final occurred = DateTime.tryParse(m.occurredAt);
      return occurred != null && occurred.isAfter(weekAgo);
    }).length;
  }

  IconData _iconFor(String type) {
    switch (type) {
      case 'knock':
        return Icons.back_hand_outlined;
      case 'signal':
        return Icons.favorite_border;
      default:
        return Icons.favorite;
    }
  }

  String _titleFor(PulseMoment m) {
    switch (m.interactionType) {
      case 'knock':
        return 'A Knock';
      case 'signal':
        return 'A Signal';
      default:
        return m.durationMs != null ? 'A Pulse — felt for ${m.durationMs}ms' : 'A Pulse';
    }
  }

  String _formatWhen(String iso) {
    final dt = DateTime.tryParse(iso);
    if (dt == null) return iso;
    final local = dt.toLocal();
    return '${local.year}-${local.month.toString().padLeft(2, '0')}-${local.day.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Text('Moments', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 4),
        Text('A private timeline of meaningful moments — never a chat history.', style: Theme.of(context).textTheme.bodySmall),
        const SizedBox(height: 16),
        if (_moments.isNotEmpty) ...[
          Text('$_thisWeekCount little moments together this week.', style: TextStyle(color: Theme.of(context).colorScheme.primary)),
          const SizedBox(height: 16),
        ],
        if (_moments.isEmpty) const Text('No moments saved yet.'),
        for (final m in _moments)
          Card(
            child: ListTile(
              leading: Icon(_iconFor(m.interactionType)),
              title: Text(_titleFor(m)),
              subtitle: Text('with @${m.otherUserId.substring(0, 8)} · ${_formatWhen(m.occurredAt)}'),
              trailing: IconButton(icon: const Icon(Icons.delete_outline), onPressed: () => _delete(m)),
            ),
          ),
        if (_error != null) ...[
          const SizedBox(height: 12),
          Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
        ],
      ],
    );
  }
}

class _PlaceholderTab extends StatelessWidget {
  final String title;
  final String subtitle;
  const _PlaceholderTab({required this.title, required this.subtitle});
  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(title, style: Theme.of(context).textTheme.headlineSmall),
              const SizedBox(height: 8),
              Text(subtitle, textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyMedium),
            ],
          ),
        ),
      );
}

/// Proves the full chain live: mobile -> pulse-api -> core-api ->
/// Postgres. Creates (or fetches, since EnsureProfile is idempotent) a
/// real pulse-profile row for the signed-in Core user.
class _ProfileTab extends StatefulWidget {
  final PulseApi pulseApi;
  final HapticEngine haptics;
  const _ProfileTab({required this.pulseApi, required this.haptics});
  @override
  State<_ProfileTab> createState() => _ProfileTabState();
}

class _ProfileTabState extends State<_ProfileTab> {
  PulseProfile? _profile;
  String? _error;

  PulsePreferences? _prefs;
  PulseQuietHours? _quietHours;
  List<PulseMute> _mutes = [];
  // Experience Controls gets its own error surface, never the profile
  // load's - a preferences hiccup shouldn't blank out the whole tab.
  String? _prefsError;
  final _quietHoursTimezone = TextEditingController(text: 'UTC');
  final _muteUserId = TextEditingController();
  int _quietHoursStartHour = 22;
  int _quietHoursEndHour = 7;

  @override
  void initState() {
    super.initState();
    _load();
    _loadExperienceControls();
  }

  @override
  void dispose() {
    _quietHoursTimezone.dispose();
    _muteUserId.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final profile = await widget.pulseApi.ensureProfile('demo_pulse');
      setState(() => _profile = profile);
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  Future<void> _loadExperienceControls() async {
    try {
      final prefs = await widget.pulseApi.getPreferences();
      final quietHours = await widget.pulseApi.getQuietHours();
      final mutes = await widget.pulseApi.listMutes();
      widget.haptics.intensity = prefs.hapticIntensity;
      setState(() {
        _prefs = prefs;
        _quietHours = quietHours;
        _quietHoursTimezone.text = quietHours.timezone;
        _quietHoursStartHour = quietHours.startMinute ~/ 60;
        _quietHoursEndHour = quietHours.endMinute ~/ 60;
        _mutes = mutes;
      });
    } catch (err) {
      setState(() => _prefsError = '$err');
    }
  }

  Future<void> _setNotificationDetail(String detail) async {
    final current = _prefs;
    if (current == null) return;
    try {
      final updated = await widget.pulseApi.setPreferences(notificationDetail: detail, hapticIntensity: current.hapticIntensity);
      setState(() => _prefs = updated);
    } catch (err) {
      setState(() => _prefsError = '$err');
    }
  }

  Future<void> _setHapticIntensity(double intensity) async {
    final current = _prefs;
    if (current == null) return;
    try {
      final updated = await widget.pulseApi.setPreferences(notificationDetail: current.notificationDetail, hapticIntensity: intensity);
      widget.haptics.intensity = updated.hapticIntensity;
      setState(() => _prefs = updated);
    } catch (err) {
      setState(() => _prefsError = '$err');
    }
  }

  Future<void> _saveQuietHours(bool enabled) async {
    try {
      final updated = await widget.pulseApi.setQuietHours(
        timezone: _quietHoursTimezone.text.trim().isEmpty ? 'UTC' : _quietHoursTimezone.text.trim(),
        startMinute: _quietHoursStartHour * 60,
        endMinute: _quietHoursEndHour * 60,
        enabled: enabled,
      );
      setState(() => _quietHours = updated);
    } catch (err) {
      setState(() => _prefsError = '$err');
    }
  }

  Future<void> _mute() async {
    final userId = _muteUserId.text.trim();
    if (userId.isEmpty) return;
    try {
      await widget.pulseApi.mute(userId);
      final mutes = await widget.pulseApi.listMutes();
      setState(() {
        _mutes = mutes;
        _muteUserId.clear();
      });
    } catch (err) {
      setState(() => _prefsError = '$err');
    }
  }

  Future<void> _unmute(String userId) async {
    try {
      await widget.pulseApi.unmute(userId);
      setState(() => _mutes = _mutes.where((m) => m.mutedUserId != userId).toList());
    } catch (err) {
      setState(() => _prefsError = '$err');
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_error != null) {
      return Center(child: Text(_error!));
    }
    final profile = _profile;
    if (profile == null) {
      return const Center(child: CircularProgressIndicator());
    }
    final prefs = _prefs;
    final quietHours = _quietHours;
    return ListView(
      padding: const EdgeInsets.all(24),
      children: [
        Text('Pulse handle', style: Theme.of(context).textTheme.labelMedium),
        Text('@${profile.handle}', style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 16),
        Text('Core User ID: ${profile.userId}'),
        Text('Created: ${profile.createdAt}'),
        const Divider(height: 40),
        Text('Experience Controls', style: Theme.of(context).textTheme.titleLarge),
        const SizedBox(height: 4),
        Text('Quiet hours, notification detail, and haptic intensity — never obligation.', style: Theme.of(context).textTheme.bodySmall),
        const SizedBox(height: 16),
        if (prefs != null) ...[
          Text('Notification detail', style: Theme.of(context).textTheme.labelLarge),
          DropdownButton<String>(
            key: const Key('notificationDetailDropdown'),
            value: prefs.notificationDetail,
            items: const [
              DropdownMenuItem(value: 'detailed', child: Text('Detailed — full content')),
              DropdownMenuItem(value: 'private', child: Text('Private — generic notification only')),
              DropdownMenuItem(value: 'silent', child: Text('Silent — no push notification')),
            ],
            onChanged: (v) {
              if (v != null) _setNotificationDetail(v);
            },
          ),
          const SizedBox(height: 16),
          Text('Haptic intensity: ${(prefs.hapticIntensity * 100).round()}%', style: Theme.of(context).textTheme.labelLarge),
          Slider(
            key: const Key('hapticIntensitySlider'),
            value: prefs.hapticIntensity,
            onChanged: (v) => setState(() => _prefs = PulsePreferences(notificationDetail: prefs.notificationDetail, hapticIntensity: v)),
            onChangeEnd: _setHapticIntensity,
          ),
        ],
        const SizedBox(height: 16),
        if (quietHours != null) ...[
          Row(
            children: [
              Text('Quiet hours', style: Theme.of(context).textTheme.labelLarge),
              const Spacer(),
              Switch(key: const Key('quietHoursEnabledSwitch'), value: quietHours.enabled, onChanged: _saveQuietHours),
            ],
          ),
          Row(
            children: [
              Expanded(
                child: TextField(key: const Key('quietHoursTimezoneField'), controller: _quietHoursTimezone, decoration: const InputDecoration(labelText: 'Timezone (IANA)')),
              ),
            ],
          ),
          Row(
            children: [
              const Text('From '),
              DropdownButton<int>(
                key: const Key('quietHoursStartHourDropdown'),
                value: _quietHoursStartHour,
                items: [for (var h = 0; h < 24; h++) DropdownMenuItem(value: h, child: Text('$h:00'))],
                onChanged: (v) => setState(() => _quietHoursStartHour = v ?? _quietHoursStartHour),
              ),
              const Text(' to '),
              DropdownButton<int>(
                key: const Key('quietHoursEndHourDropdown'),
                value: _quietHoursEndHour,
                items: [for (var h = 0; h < 24; h++) DropdownMenuItem(value: h, child: Text('$h:00'))],
                onChanged: (v) => setState(() => _quietHoursEndHour = v ?? _quietHoursEndHour),
              ),
              const SizedBox(width: 8),
              TextButton(key: const Key('saveQuietHoursButton'), onPressed: () => _saveQuietHours(quietHours.enabled), child: const Text('Save')),
            ],
          ),
        ],
        const Divider(height: 40),
        Text('Muted', style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 4),
        Text('Muting is silent and one-directional — the muted person is never told.', style: Theme.of(context).textTheme.bodySmall),
        const SizedBox(height: 8),
        Row(
          children: [
            Expanded(child: TextField(key: const Key('muteUserIdField'), controller: _muteUserId, decoration: const InputDecoration(labelText: 'User ID to mute'))),
            const SizedBox(width: 8),
            FilledButton(key: const Key('muteButton'), onPressed: _mute, child: const Text('Mute')),
          ],
        ),
        for (final m in _mutes)
          ListTile(
            title: Text(m.mutedUserId),
            trailing: TextButton(onPressed: () => _unmute(m.mutedUserId), child: const Text('Unmute')),
          ),
        if (_prefsError != null) ...[
          const SizedBox(height: 12),
          Text(_prefsError!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
        ],
      ],
    );
  }
}
