import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:core_sdk/core_sdk.dart';

import 'pulse_api.dart';

const _keycloakUrl = String.fromEnvironment('KEYCLOAK_URL', defaultValue: 'http://localhost:8081');
const _coreApiUrl = String.fromEnvironment('CORE_API_URL', defaultValue: 'http://localhost:8080');
const _pulseApiUrl = String.fromEnvironment('PULSE_API_URL', defaultValue: 'http://localhost:8096');
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
  final String realm;
  final String clientId;
  final http.Client? httpClient;

  const LoginPage({
    super.key,
    this.keycloakUrl = _keycloakUrl,
    this.coreApiUrl = _coreApiUrl,
    this.pulseApiUrl = _pulseApiUrl,
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
        MaterialPageRoute(builder: (_) => HomeShell(coreApi: coreApi, pulseApi: pulseApi)),
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
/// Mood, Moments, Profile. Every tab but Profile is a placeholder for
/// now; only pulse-profile exists as a real backend module (Phase 1),
/// so Profile is the one tab that proves the whole chain end to end.
class HomeShell extends StatefulWidget {
  final CoreApi coreApi;
  final PulseApi pulseApi;
  const HomeShell({super.key, required this.coreApi, required this.pulseApi});

  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _tab = 0;

  @override
  Widget build(BuildContext context) {
    final pages = [
      const _HomeTab(),
      const _PlaceholderTab(title: 'People', subtitle: 'Partner Bond, Close Friends, Friends, Circles — Phase 2'),
      const _PlaceholderTab(title: 'Mood', subtitle: "Today's Mood — Phase 8"),
      const _PlaceholderTab(title: 'Moments', subtitle: 'Saved shared moments, no chat — Phase 12'),
      _ProfileTab(pulseApi: widget.pulseApi),
    ];
    return Scaffold(
      body: SafeArea(child: pages[_tab]),
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

/// The "hold to Pulse" main screen from product spec §13 - visual only
/// for now (pulse-interactions, the module that turns a hold into a
/// real felt gesture, is Phase 4, not Phase 1).
class _HomeTab extends StatelessWidget {
  const _HomeTab();
  @override
  Widget build(BuildContext context) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 160,
              height: 160,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: Theme.of(context).colorScheme.primary.withValues(alpha: 0.15),
                border: Border.all(color: Theme.of(context).colorScheme.primary, width: 2),
              ),
              child: Icon(Icons.favorite, size: 64, color: Theme.of(context).colorScheme.primary),
            ),
            const SizedBox(height: 16),
            const Text('Hold to Pulse'),
            const SizedBox(height: 4),
            Text('Coming in Phase 4', style: Theme.of(context).textTheme.bodySmall),
          ],
        ),
      );
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
