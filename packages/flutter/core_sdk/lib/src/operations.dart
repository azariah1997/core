import 'client.dart';
import 'pagination.dart';

/// Typed convenience methods layered on CoreClient.request, covering
/// the same representative "core identity" slice the Go and TypeScript
/// SDKs do (platform, identity, users, devices, applications) - not all
/// 146 registered routes. Every other route is still reachable via the
/// inherited request() method; hand-typing more wrappers is real,
/// additive work anyone can do the same way these were written (copy a
/// *http.go response struct's JSON shape verbatim - never guessed), not
/// a framework limitation.
class CoreApi extends CoreClient {
  CoreApi(super.baseUrl, {super.tokenSource, super.httpClient, super.retryPolicy});

  Future<Platform> getPlatform() => request('GET', '/v1/platform', decode: (j) => Platform.fromJson(j));

  Future<Identity> identityMe() => request('GET', '/v1/identity/me', decode: (j) => Identity.fromJson(j));

  Future<User> usersMe() => request('GET', '/v1/users/me', decode: (j) => User.fromJson(j));

  Future<User> usersUpdateMe(UpdateUserInput input) =>
      request('PATCH', '/v1/users/me', body: input.toJson(), decode: (j) => User.fromJson(j));

  Future<User> usersGet(String id) => request('GET', '/v1/users/$id', decode: (j) => User.fromJson(j));

  /// platform.admin-only (Phase 25's admin-wide listing).
  Future<Page<User>> usersList({String? cursor}) => request(
        'GET',
        '/v1/users',
        query: {'cursor': cursor},
        decode: (j) => Page<User>(
          (j['items'] as List).map((e) => User.fromJson(e as Map<String, dynamic>)).toList(),
          j['nextCursor'] as String?,
        ),
      );

  /// The roadmap names "device registration" as its own SDK
  /// responsibility, distinct from generic "API calls," since a
  /// real-time connection (see RealtimeClient.dial) needs a registered
  /// device id first.
  Future<Device> devicesRegister(RegisterDeviceInput input) =>
      request('POST', '/v1/users/me/devices', body: input.toJson(), decode: (j) => Device.fromJson(j));

  Future<List<Device>> devicesList() => request(
        'GET',
        '/v1/users/me/devices',
        decode: (j) => (j['items'] as List).map((e) => Device.fromJson(e as Map<String, dynamic>)).toList(),
      );

  Future<void> devicesRevoke(String id) => request('DELETE', '/v1/users/me/devices/$id');

  Future<Application> applicationsCreate(CreateApplicationInput input) =>
      request('POST', '/v1/apps', body: input.toJson(), decode: (j) => Application.fromJson(j));

  Future<Page<Application>> applicationsList({String? cursor}) => request(
        'GET',
        '/v1/apps',
        query: {'cursor': cursor},
        decode: (j) => Page<Application>(
          (j['items'] as List).map((e) => Application.fromJson(e as Map<String, dynamic>)).toList(),
          j['nextCursor'] as String?,
        ),
      );

  Future<Application> applicationsGet(String id) => request('GET', '/v1/apps/$id', decode: (j) => Application.fromJson(j));
}

// --- response/request shapes, each copied verbatim from the owning
// module's real *http.go response struct - never guessed. ---

class Platform {
  final String name;
  final String environment;
  final String apiVersion;
  Platform({required this.name, required this.environment, required this.apiVersion});
  factory Platform.fromJson(Map<String, dynamic> j) =>
      Platform(name: j['name'] as String, environment: j['environment'] as String, apiVersion: j['apiVersion'] as String);
}

class Identity {
  final String id;
  final String provider;
  final String providerSubject;
  final String status;
  final String? userId;
  Identity({required this.id, required this.provider, required this.providerSubject, required this.status, this.userId});
  factory Identity.fromJson(Map<String, dynamic> j) => Identity(
        id: j['id'] as String,
        provider: j['provider'] as String,
        providerSubject: j['providerSubject'] as String,
        status: j['status'] as String,
        userId: j['userId'] as String?,
      );
}

class User {
  final String id;
  final String displayName;
  final String? avatarRef;
  final String locale;
  final String timezone;
  final String status;
  final String createdAt;
  final String updatedAt;
  User({
    required this.id,
    required this.displayName,
    this.avatarRef,
    required this.locale,
    required this.timezone,
    required this.status,
    required this.createdAt,
    required this.updatedAt,
  });
  factory User.fromJson(Map<String, dynamic> j) => User(
        id: j['id'] as String,
        displayName: j['displayName'] as String,
        avatarRef: j['avatarRef'] as String?,
        locale: j['locale'] as String,
        timezone: j['timezone'] as String,
        status: j['status'] as String,
        createdAt: j['createdAt'] as String,
        updatedAt: j['updatedAt'] as String,
      );
}

class UpdateUserInput {
  final String? displayName;
  final String? avatarRef;
  final String? locale;
  final String? timezone;
  final String? status;
  UpdateUserInput({this.displayName, this.avatarRef, this.locale, this.timezone, this.status});
  Map<String, dynamic> toJson() => {
        if (displayName != null) 'displayName': displayName,
        if (avatarRef != null) 'avatarRef': avatarRef,
        if (locale != null) 'locale': locale,
        if (timezone != null) 'timezone': timezone,
        if (status != null) 'status': status,
      };
}

class Device {
  final String id;
  final String clientDeviceId;
  final String platform;
  final String? osVersion;
  final String? appVersion;
  final String locale;
  final String timezone;
  final bool hasPushToken;
  final String sessionStatus;
  final String lastActiveAt;
  final String createdAt;
  Device({
    required this.id,
    required this.clientDeviceId,
    required this.platform,
    this.osVersion,
    this.appVersion,
    required this.locale,
    required this.timezone,
    required this.hasPushToken,
    required this.sessionStatus,
    required this.lastActiveAt,
    required this.createdAt,
  });
  factory Device.fromJson(Map<String, dynamic> j) => Device(
        id: j['id'] as String,
        clientDeviceId: j['clientDeviceId'] as String,
        platform: j['platform'] as String,
        osVersion: j['osVersion'] as String?,
        appVersion: j['appVersion'] as String?,
        locale: j['locale'] as String,
        timezone: j['timezone'] as String,
        hasPushToken: j['hasPushToken'] as bool,
        sessionStatus: j['sessionStatus'] as String,
        lastActiveAt: j['lastActiveAt'] as String,
        createdAt: j['createdAt'] as String,
      );
}

class RegisterDeviceInput {
  final String clientDeviceId;
  final String platform;
  final String? osVersion;
  final String? appVersion;
  final String? locale;
  final String? timezone;
  final String? pushToken;
  RegisterDeviceInput({
    required this.clientDeviceId,
    required this.platform,
    this.osVersion,
    this.appVersion,
    this.locale,
    this.timezone,
    this.pushToken,
  });
  Map<String, dynamic> toJson() => {
        'clientDeviceId': clientDeviceId,
        'platform': platform,
        if (osVersion != null) 'osVersion': osVersion,
        if (appVersion != null) 'appVersion': appVersion,
        if (locale != null) 'locale': locale,
        if (timezone != null) 'timezone': timezone,
        if (pushToken != null) 'pushToken': pushToken,
      };
}

class Application {
  final String id;
  final String slug;
  final String name;
  final String status;
  final String createdAt;
  final String updatedAt;
  Application({
    required this.id,
    required this.slug,
    required this.name,
    required this.status,
    required this.createdAt,
    required this.updatedAt,
  });
  factory Application.fromJson(Map<String, dynamic> j) => Application(
        id: j['id'] as String,
        slug: j['slug'] as String,
        name: j['name'] as String,
        status: j['status'] as String,
        createdAt: j['createdAt'] as String,
        updatedAt: j['updatedAt'] as String,
      );
}

class CreateApplicationInput {
  final String slug;
  final String name;
  CreateApplicationInput({required this.slug, required this.name});
  Map<String, dynamic> toJson() => {'slug': slug, 'name': name};
}
