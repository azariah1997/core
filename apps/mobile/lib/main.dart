import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:core_sdk/core_sdk.dart';

const _keycloakUrl = String.fromEnvironment('KEYCLOAK_URL', defaultValue: 'http://localhost:8081');
const _coreApiUrl = String.fromEnvironment('CORE_API_URL', defaultValue: 'http://localhost:8080');
const _realm = String.fromEnvironment('KEYCLOAK_REALM', defaultValue: 'core');
const _clientId = String.fromEnvironment('KEYCLOAK_CLIENT_ID', defaultValue: 'core-platform');

void main() => runApp(const CoreDemoApp());

class CoreDemoApp extends StatelessWidget {
  const CoreDemoApp({super.key});
  @override
  Widget build(BuildContext context) => MaterialApp(
        title: 'Core Platform',
        theme: ThemeData(useMaterial3: true, colorSchemeSeed: Colors.indigo),
        home: const LoginPage(),
      );
}

/// A real, if minimal, proof that apps/mobile consumes core_sdk (Phase
/// 27) rather than talking to core-api directly - login mints a real
/// Keycloak token via PasswordTokenSource, then CoreApi fetches the
/// caller's real profile and registers this device, the same "core
/// identity" slice the Go and TypeScript SDKs demonstrate.
class LoginPage extends StatefulWidget {
  // Overridable so tests can point at a different address and, since
  // Flutter's widget-test binding forces every real HttpClient request
  // to fail with a 400 by design (discovered live - see
  // apps/mobile/test/login_page_test.dart's own comment), inject a
  // fake http.Client instead - the framework's own documented way to
  // test HTTP-driven widgets, distinct from packages/flutter/core_sdk's
  // own unit tests, which use a real local server under plain `dart
  // test` (unaffected by this Flutter-specific constraint).
  final String keycloakUrl;
  final String coreApiUrl;
  final String realm;
  final String clientId;
  final http.Client? httpClient;

  const LoginPage({
    super.key,
    this.keycloakUrl = _keycloakUrl,
    this.coreApiUrl = _coreApiUrl,
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
    final api = CoreApi(widget.coreApiUrl, tokenSource: tokens, httpClient: widget.httpClient);
    try {
      // Fails fast with a real ApiError/auth error if the credentials
      // or the token grant itself are bad, before navigating anywhere.
      await api.identityMe();
      if (!mounted) return;
      Navigator.of(context).pushReplacement(
        MaterialPageRoute(builder: (_) => ProfilePage(api: api, tokens: tokens)),
      );
    } catch (err) {
      setState(() => _error = '$err');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) => Scaffold(
        appBar: AppBar(title: const Text('Core Platform')),
        body: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text('Sign in with your platform account (Keycloak realm "${widget.realm}").', style: Theme.of(context).textTheme.bodyMedium),
              const SizedBox(height: 16),
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
      );
}

class ProfilePage extends StatefulWidget {
  final CoreApi api;
  final TokenSource tokens;
  const ProfilePage({super.key, required this.api, required this.tokens});
  @override
  State<ProfilePage> createState() => _ProfilePageState();
}

class _ProfilePageState extends State<ProfilePage> {
  User? _me;
  Device? _device;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final me = await widget.api.usersMe();
      final device = await widget.api.devicesRegister(
        RegisterDeviceInput(clientDeviceId: 'core-mobile-demo', platform: 'flutter'),
      );
      setState(() {
        _me = me;
        _device = device;
      });
    } catch (err) {
      setState(() => _error = '$err');
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_error != null) {
      return Scaffold(appBar: AppBar(title: const Text('Core Platform')), body: Center(child: Text(_error!)));
    }
    final me = _me;
    if (me == null) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    return Scaffold(
      appBar: AppBar(title: const Text('Core Platform')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Signed in as', style: Theme.of(context).textTheme.labelMedium),
            Text(me.displayName, style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: 16),
            Text('User ID: ${me.id}'),
            Text('Locale: ${me.locale}'),
            Text('Timezone: ${me.timezone}'),
            const SizedBox(height: 16),
            if (_device != null) Text('Registered device: ${_device!.id}'),
          ],
        ),
      ),
    );
  }
}
