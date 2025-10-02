# Model Pretnji - Mini-Zanzibar Sistem Autorizacije

## Pregled Sistema

Mini-Zanzibar je globalni sistem autorizacije koji upravlja kontrolom pristupa kroz:
- LevelDB za čuvanje ACL torki
- ConsulDB za konfiguraciju namespace-a sa verzionisanjem
- REST API za upravljanje ACL-om i namespace-om

## Sredstva

### Primarna Sredstva:
1. **Podaci o autorizaciji (ACL torke)** - Kritično  
   - Format: objekat#relacija@korisnik  
   - Čuvaju se u LevelDB  
   - Kontrolišu pristup zaštićenim resursima  

2. **Namespace Konfiguracije** - Visoko  
   - Definišu pravila i relacije autorizacije  
   - Čuvaju se u ConsulDB sa verzionisanjem  
   - Kontrolišu logiku autorizacije  

3. **API Servis** - Visoko  
   - Pruža odluke o autorizaciji  
   - Upravljа ACL i namespace operacijama  

### Pomoćna Sredstva:
1. **Baze podataka** (LevelDB, ConsulDB)  
2. **Konfiguracioni podaci**  
3. **Log fajlovi**  
4. **Sistemska infrastruktura**  

## Akteri Pretnji

### Spoljni Napadači
- **Nivo**: Srednji do visok  
- **Motivacija**: Krađa podataka, ometanje sistema, eskalacija privilegija  
- **Pristup**: Mrežni pristup API endpoint-ima  

### Zlonamerni Insajderi
- **Nivo**: Visok  
- **Motivacija**: Krađa podataka, sabotaža, neautorizovan pristup  
- **Pristup**: Potencijalni sistemski pristup  

### Kompromitovane Aplikacije
- **Nivo**: Varijabilan  
- **Motivacija**: Lateralno kretanje, pristup podacima  
- **Pristup**: API pristup kroz kompromitovane klijentske aplikacije  

## Vektori Napada i Pretnje

### 1. Pretnje API Bezbednosti

#### T1.1: Neautorizovan API Pristup
- **Opis**: Napadači zaobilaze autentifikaciju da bi pristupili API endpoint-ima  
- **Uticaj**: Visok - Neautorizovane izmene ACL-a, izlaganje podataka  
- **Verovatnoća**: Srednja  
- **Rešenja**:  
  - TODO: Implementirati JWT autentifikaciju  
  - TODO: Ograničavanje broja zahteva (rate limiting)  
  - TODO: Validacija zahteva  

#### T1.2: Injekcioni Napadi
- **Opis**: SQL/NoSQL injekcije kroz API parametre  
- **Uticaj**: Visok - Kompromitovanje baze, manipulacija podacima  
- **Verovatnoća**: Srednja  
- **Rešenja**:  
  - TODO: Validacija i sanitizacija ulaza  
  - TODO: Parametrizovani upiti  
  - TODO: Kontrola pristupa bazi  

#### T1.3: Zaobilaženje Autorizacije
- **Opis**: Greške u logici autorizacije omogućavaju eskalaciju privilegija  
- **Uticaj**: Kritičan – Potpuno zaobilaženje kontrole pristupa  
- **Verovatnoća**: Visoka  
- **Rešenja**:  
  - TODO: Sveobuhvatno testiranje autorizacije  
  - TODO: Bezbednosne prakse u kodiranju  
  - TODO: Redovni bezbednosni pregledi  

### 2. Pretnje Skladištenja Podataka

#### T2.1: Kompromitovanje Baze
- **Opis**: Direktan pristup LevelDB ili ConsulDB  
- **Uticaj**: Kritičan – Potpuno izlaganje/manipulacija ACL podataka  
- **Verovatnoća**: Srednja  
- **Rešenja**:  
  - TODO: Enkripcija baze u stanju mirovanja  
  - TODO: Kontrola pristupa i monitoring  
  - TODO: Mrežna segmentacija  

#### T2.2: Manipulacija Konfiguracijom
- **Opis**: Neautorizovana izmena namespace konfiguracija  
- **Uticaj**: Visok – Manipulacija logikom autorizacije  
- **Verovatnoća**: Srednja  
- **Rešenja**:  
  - TODO: Verzije konfiguracije i provere integriteta  
  - TODO: Administrativna kontrola pristupa  
  - TODO: Revizija izmena  

### 3. Pretnje Mrežne Bezbednosti

#### T3.1: Man-in-the-Middle Napadi
- **Opis**: Presretanje API komunikacije  
- **Uticaj**: Visok – Krađa kredencijala, izlaganje podataka  
- **Verovatnoća**: Srednja  
- **Rešenja**:  
  - TODO: TLS enkripcija za sve komunikacije  
  - TODO: Validacija sertifikata  
  - TODO: Mrežni monitoring  

#### T3.2: Denial of Service
- **Opis**: Napadi iscrpljivanja resursa na API endpoint-ima  
- **Uticaj**: Srednji – Nedostupnost servisa  
- **Verovatnoća**: Visoka  
- **Rešenja**:  
  - TODO: Implementacija rate limiting-a  
  - TODO: Praćenje resursa  
  - TODO: Load balancing  

### 4. Pretnje Logike Aplikacije

#### T4.1: Race Conditions
- **Opis**: Istovremene ACL operacije izazivaju nedosledno stanje  
- **Uticaj**: Srednji – Nedoslednost podataka, privremena eskalacija privilegija  
- **Verovatnoća**: Srednja  
- **Rešenja**:  
  - TODO: Kontrole konkurentnosti  
  - TODO: Transakcije baze  
  - TODO: Validacija stanja  

#### T4.2: Greške u Poslovnoj Logici
- **Opis**: Greške u logici evaluacije ACL-a  
- **Uticaj**: Visok – Pogrešne odluke o autorizaciji  
- **Verovatnoća**: Srednja  
- **Rešenja**:  
  - TODO: Sveobuhvatno testiranje  
  - TODO: Code review  
  - TODO: Formalne metode verifikacije  

## Procesni Modeli

### Proces Kreiranja ACL-a
1. **Ulaz**: API zahtev sa objektom, relacijom, korisnikom  
2. **Validacija**: Provera formata, autentifikacija  
3. **Autorizacija**: Provera dozvola za upravljanje ACL-om  
4. **Skladištenje**: Čuvanje torke u LevelDB  
5. **Logovanje**: Revizija u logovima  
6. **Odgovor**: Potvrda ili greška  

**Pretnje**: Neautorizovano kreiranje, injekcioni napadi, korupcija podataka  

### Proces Provere Autorizacije
1. **Ulaz**: API zahtev sa objektom, relacijom, korisnikom  
2. **Validacija**: Provera formata  
3. **Pretraga Namespace-a**: Dohvatanje konfiguracije relacije iz Consul-a  
4. **Evaluacija**: Primena pravila autorizacije (direktni, računati korisnički setovi)  
5. **Keširanje**: Čuvanje rezultata radi performansi (TODO)  
6. **Odgovor**: Odluka o autorizaciji  

**Pretnje**: Zaobilaženje logike, trovanje keša, napadi na performanse  

### Proces Upravljanja Namespace-om
1. **Ulaz**: API zahtev sa konfiguracijom namespace-a  
2. **Validacija**: Validacija formata i logike konfiguracije  
3. **Autorizacija**: Provera administratorskog pristupa  
4. **Verzionisanje**: Kreiranje nove verzije u Consul-u  
5. **Aktivacija**: Ažuriranje pokazivača na poslednju verziju  
6. **Logovanje**: Revizija promena konfiguracije  

**Pretnje**: Manipulacija konfiguracijom, eskalacija privilegija, greške u logici  

## Matrica Procene Rizika

| Pretnja | Uticaj | Verovatnoća | Nivo Rizika | Prioritet |
|---------|---------|-------------|-------------|-----------|
| T1.3 - Zaobilaženje Autorizacije | Kritičan | Visoka | Kritičan | 1 |
| T2.1 - Kompromitovanje Baze | Kritičan | Srednja | Visok | 2 |
| T1.1 - Neautorizovan API Pristup | Visok | Srednja | Visok | 3 |
| T1.2 - Injekcioni Napadi | Visok | Srednja | Visok | 4 |
| T2.2 - Manipulacija Konfiguracijom | Visok | Srednja | Visok | 5 |
| T3.1 - MITM Napadi | Visok | Srednja | Visok | 6 |
| T4.2 - Greške u Poslovnoj Logici | Visok | Srednja | Visok | 7 |
| T4.1 - Uslovi Utrke | Srednji | Srednja | Srednji | 8 |
| T3.2 - Denial of Service | Srednji | Visoka | Srednji | 9 |

## Preporuke

### Visok Prioritet:
1. Implementirati sveobuhvatnu logiku autorizacije sa adekvatnim testiranjem  
2. Dodati autentifikaciju i autorizacioni middleware  
3. Implementirati validaciju i sanitizaciju ulaza  
4. Dodati enkripciju baze i kontrole pristupa  

### Srednji Prioritet:
1. Implementirati TLS za sve komunikacije  
2. Dodati sveobuhvatno revizijsko logovanje  
3. Implementirati rate limiting i DDoS zaštitu  
4. Dodati provere integriteta konfiguracije  

### Nizak Prioritet:
1. Implementirati keširanje sa bezbednosnim razmatranjima  
2. Dodati monitoring i upozoravanje  
3. Implementirati procedure za backup i oporavak  
4. Dodati testiranje performansi i optimizaciju  