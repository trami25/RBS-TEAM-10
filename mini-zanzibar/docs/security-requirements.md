# **Bezbednosni zahtevi i analiza OWASP Top 10**

Ovaj dokument opisuje bezbednosne zahteve za Mini-Zanzibar sistem autorizacije zasnovan na OWASP Application Security Verification Standard (ASVS) i pruža sveobuhvatnu analizu u odnosu na OWASP Top 10 iz 2021\. godine.

## **OWASP Top 10 2021 Analiza**

### **A01: Neispravna kontrola pristupa**

**Pronađene ranjivosti:**

* Nedovoljna autorizacija na frontendu

* Bob nije mogao da upravlja ACL-ovima za dokumente koje poseduje

* Nedostajala validacija autorizacije u proxy rutiranju

**Implementirane ispravke:**

* Poboljšano upravljanje ACL-ovima uz validaciju vlasništva

* Ispravljeno proxy rutiranje auth servisa ka Mini-Zanzibaru

* Automatsko dodeljivanje vlasništva (Alice) za nove dokumente

* Middleware za autorizaciju na svim zaštićenim endpointima

Svi API endpointi su. zaštićeni autentikacijom i autorizacijom

### **A02: Kriptografski propusti**

**Pronađene ranjivosti:**

* Lozinke čuvane u običnom tekstu

* Hardkodovan JWT secret: "your-secret-key-change-in-production"

* Hardkodovan session secret: "secret-key-change-in-production"

* Nesigurna konfiguracija sesije

**Implementirane ispravke:**

* Bcrypt heširanje lozinki za sve korisnike

* JWT\_SECRET postavljen kroz environment varijable

* SESSION\_SECRET postavljen kroz environment varijable

* Sigurna podešavanja sesije (HttpOnly, Secure, SameSite: Strict)

### **A03: Injekcija**

**Pronađene ranjivosti:**

* Nedovoljna validacija inputa na JSON endpointima

* Mogućnost obrade neispravnih zahteva

**Implementirane ispravke:**

* Middleware za sveobuhvatnu validaciju ulaza

* Ispravno parsiranje JSON-a sa hvatanjem grešaka

* Čišćenje i validacija zahteva

Svi ulazi su pravilno validirani i očišćeni.

### **A04: Nesiguran dizajn**

**Problematične oblasti:**

* Nedostatak formalne dokumentacije o modelovanju pretnji

* Ograničen pregled bezbednosne arhitekture

* Nedostaje dokumentacija o bezbednosnim obrascima dizajna

**Preporuke:**

* Uvesti formalno modelovanje pretnji \- urađen threat modeling.  
* Redovno raditi bezbednosne preglede dizajna \- implementirano skeniranje softvera Github CodeQL alatom za statičku analizu koda

### **A05: Pogrešna konfiguracija bezbednosti**

**Pronađene ranjivosti:**

* Nedostajale bezbednosne HTTP zaglavlja

* Previše permisivna CORS konfiguracija

* Nema mehanizma za HTTPS primoravanje

* Nesigurne podrazumevane konfiguracije

**Implementirane ispravke:**

* Middleware za bezbednosna zaglavlja (X-Frame-Options, X-Content-Type-Options, X-XSS-Protection, Referrer-Policy, Content-Security-Policy)

* Restriktivna CORS konfiguracija

* Podrazumevana bezbednosna podešavanja za produkciju

* Konfiguracije specifične po okruženju

Sve konfiguracije su bezbednosti pravilno implementirane.

### **A06: Ranjive i zastarele komponente**

**Trenutni status:**

* Potrebno redovno skeniranje zavisnosti na ranjivosti

* Nema automatizovanog procesa ažuriranja zavisnosti

**Preporuke:**

* Implementirati alate za skeniranje ranjivosti (Snyk, OWASP Dependency Check)

* Postaviti automatizovana ažuriranja zavisnosti

* Redovno raditi bezbednosne revizije third-party komponenti

### **A07: Neuspeh identifikacije i autentikacije**

**Pronađene ranjivosti:**

* Nema ograničenja pokušaja logovanja

* Slabo upravljanje sesijama

* Nema zaštite od brute force napada

**Implementirane ispravke:**

* Rate limiting: 5 pokušaja logina po IP adresi u minuti

* Sigurna konfiguracija sesija sa time-out-om od 15 minuta

* Jaka bcrypt verifikacija lozinki

* Ispravno upravljanje autentikacionim stanjem

Implementiran sistem autentikacije i sesija.

### **A08: Propusti u integritetu softvera i podataka**

**Potrebno:**

* Potpisivanje koda za release verzije

* Provere integriteta kritičnih podataka

* CI/CD pipeline sa bezbednosnim skeniranjem \- (Redis baza izolovana u Docker-u)

### **A09: Neuspešno logovanje i monitoring**

**Trenutno:**

* Implementirano osnovno aplikaciono logovanje pomoću zap

**Mogućnosti za dodatno unapređenje sistema:**

* Uvesti logovanje bezbednosnih događaja

* Postaviti real-time monitoring i alerting

* Analiza i korelacija logova radi otkrivanja pretnji

### **A10: Server-Side Request Forgery (SSRF)**

**Problematične oblasti:**

* Proxy funkcionalnost između auth-servisa i mini-zanzibara

* Nedostaje validacija URL-ova za spoljne zahteve

**Preporuke:**

* Uvesti validaciju i listu dozvoljenih URL-ova

* Revidirati proxy implementaciju radi SSRF ranjivosti

* Dodati mrežne kontrole

## **Rezime \- Tabela**

| OWASP kategorija | Nivo rizika | Status | Implementacija |
| ----- | ----- | ----- | ----- |
| A01: Broken Access Control | VISOK → NIZAK | BEZBEDNO | Završeno |
| A02: Cryptographic Failures | KRITIČAN → NIZAK | BEZBEDNO | Završeno |
| A03: Injection | SREDNJI → NIZAK | BEZBEDNO | Završeno |
| A04: Insecure Design | SREDNJI | BEZBEDNO | Završeno |
| A05: Security Misconfiguration | VISOK → NIZAK | BEZBEDNO | Završeno |
| A06: Vulnerable Components | SREDNJI | PROVERA | Potrebna provera |
| A07: Auth/Session Failures | KRITIČAN → NIZAK | BEZBEDNO | Završeno |
| A08: Data Integrity | SREDNJI | DELIMIČNO | Potrebno doraditi |
| A09: Logging/Monitoring | SREDNJI | DELIMIČNO | Potrebno doraditi |
| A10: SSRF | SREDNJI | PROVERA | Potrebna provera |
