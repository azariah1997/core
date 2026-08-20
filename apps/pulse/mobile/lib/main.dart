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

  @override
  void initState() {
    super.initState();
    _connectRealtime();
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
      _HomeTab(pulseApi: widget.pulseApi, haptics: _haptics),
      const _PlaceholderTab(title: 'People', subtitle: 'Partner Bond, Close Friends, Friends, Circles — Phase 9'),
      const _PlaceholderTab(title: 'Mood', subtitle: "Today's Mood — Phase 8"),
      const _PlaceholderTab(title: 'Moments', subtitle: 'Saved shared moments, no chat — Phase 12'),
      _ProfileTab(pulseApi: widget.pulseApi),
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

/// The "hold to Pulse" main screen (product spec §13, Phase 4). Press
/// down starts a real Pulse (create+start in one call); release stops
/// it - PulseStart/PulseStop, never a continuous stream (spec §15).
/// Duration shown to the sender is local (for feel); the server's own
/// duration (spec §78, never trusted from the client) is what's
/// actually recorded.
class _HomeTab extends StatefulWidget {
  final PulseApi pulseApi;
  final HapticEngine haptics;
  const _HomeTab({required this.pulseApi, required this.haptics});

  @override
  State<_HomeTab> createState() => _HomeTabState();
}

class _HomeTabState extends State<_HomeTab> {
  List<PulseConnection> _connections = [];
  PulseConnection? _target;
  String? _error;
  bool _loadingConnections = true;

  bool _holding = false;
  String? _activeInteractionId;
  DateTime? _holdStartedAt;
  Duration _elapsed = Duration.zero;
  Timer? _ticker;
  String? _lastResult;

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
    } catch (err) {
      setState(() {
        _error = '$err';
        _loadingConnections = false;
      });
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
      setState(() => _lastResult = 'Pulse sent — felt for ${stopped.durationMs ?? 0}ms');
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
      await widget.pulseApi.knock(target.otherUserId);
      setState(() => _lastResult = 'Knock sent');
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
                          onSelected: _holding ? null : (_) => setState(() => _target = c),
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
          Text('to @${target.otherUserId.substring(0, 8)} (${target.classification})', style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 12),
          OutlinedButton(
            key: const Key('knockButton'),
            onPressed: _holding ? null : _sendKnock,
            child: const Text('Knock'),
          ),
          if (_lastResult != null) ...[
            const SizedBox(height: 12),
            Text(_lastResult!, style: TextStyle(color: Theme.of(context).colorScheme.primary)),
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
  const _ProfileTab({required this.pulseApi});
  @override
  State<_ProfileTab> createState() => _ProfileTabState();
}

class _ProfileTabState extends State<_ProfileTab> {
  PulseProfile? _profile;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final profile = await widget.pulseApi.ensureProfile('demo_pulse');
      setState(() => _profile = profile);
    } catch (err) {
      setState(() => _error = '$err');
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
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Pulse handle', style: Theme.of(context).textTheme.labelMedium),
          Text('@${profile.handle}', style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 16),
          Text('Core User ID: ${profile.userId}'),
          Text('Created: ${profile.createdAt}'),
        ],
      ),
    );
  }
}
