import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;

void main() => runApp(const CoreDemoApp());

class CoreDemoApp extends StatelessWidget {
  const CoreDemoApp({super.key});
  @override Widget build(BuildContext context) => MaterialApp(home: const Home());
}

class Home extends StatefulWidget { const Home({super.key}); @override State<Home> createState()=>_HomeState(); }
class _HomeState extends State<Home>{
  String status='Not checked';
  Future<void> check() async { try { final r=await http.get(Uri.parse('http://localhost:8080/healthz')); setState(()=>status='API ${r.statusCode}: ${r.body}'); } catch(e){ setState(()=>status='API unavailable: $e'); } }
  @override Widget build(BuildContext context)=>Scaffold(appBar:AppBar(title:const Text('Core Platform Demo')),body:Padding(padding:const EdgeInsets.all(24),child:Column(crossAxisAlignment:CrossAxisAlignment.start,children:[Text(status),const SizedBox(height:16),FilledButton(onPressed:check,child:const Text('Check Core API'))])));
}
