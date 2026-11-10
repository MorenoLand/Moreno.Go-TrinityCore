-- Eredar Twins (Sunwell Plateau) — Lua port of
-- src/server/scripts/EasternKingdoms/SunwellPlateau/boss_eredar_twins.cpp
-- (boss_sacrolash, boss_alythess, npc_shadow_image); sunwell_
-- plateau.h:34 (DATA_EREDAR_TWINS = 3, fourth boss), :42 (DATA_
-- ALYTHESS), :43 (DATA_SACROLASH), :76 (NPC_GRAND_WARLOCK_ALYTHESS
-- = 25166), :77 (NPC_SHADOW_IMAGE = 25214), :78 (NPC_LADY_
-- SACROLASH = 25165). Entries: 25165 Lady Sacrolash (C++ ScriptName
-- "boss_sacrolash"), 25166 Grand Warlock Alythess (C++ ScriptName
-- "boss_alythess"), 25214 Shadow Image (C++ ScriptName "npc_
-- shadow_image") — all entry+ScriptName verifiable from the C++
-- sources per RegisterSunwellPlateauCreatureAI in AddSC_boss_
-- eredar_twins; the creature_template ScriptName bindings are DB-
-- side — no TDB in this workspace, so only the C++-side naming is
-- verified. Talk lines used: Sacrolash YELL_SAC_KILL=8 (kill, 25%),
-- EMOTE_SHADOW_NOVA=5 + YELL_SHADOW_NOVA=9 (shadow nova);
-- Alythess EMOTE_CONFLAGRATION=4 + YELL_CANFLAGRATION=8
-- (conflagration), YELL_ALY_KILL=5 (kill, 25%), YELL_BERSERK=9
-- (berserk). YELL_INTRO_SAC_*=0/1/2/3 and YELL_INTRO_ALY_*=0/1/2/3
-- belong to the unmodeled intro chain; YELL_SISTER_ALYTHESS_DEAD=7
-- and YELL_SISTER_SACROLASH_DEAD=7 belong to the sister-death
-- detection arms, PORTED 2026-11-05 via the GetInstanceCreature
-- bridge + 1s in-combat poll (below). YELL_SAC_DEAD=4
-- and YELL_ALY_DEAD=6 are the JustDied death yells, ported via the
-- twinDead latch (see below).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. CREATURE_EVENT_ON_SPELL_HIT_TARGET (15) is
-- bridged and fires once per ACTUAL hit unit of each cast — primary,
-- chain jumps, AoE fan-out (boss_ai.go
-- castCreatureSpellInternalWithBasePoints: hitTargets is the primary
-- target plus the SelectImplicitAreaTargets / cone sweeps, and the
-- per-target loop fires ev15 per guid; the arugal precedent). The
-- SpellHitTarget touched-spell arms are PORTED below via ev15 handlers
-- (the 2026-10-17 "Go reports the cast's single target" rationale is
-- stale). Self-cast AoEs list the caster as hitTargets[0], so the
-- handlers skip the caster herself — C++ never fires SpellHitTarget on
-- the caster of her own enemy-AoE.
-- Fight shape (C++-exact for the modeled arms): Lady Sacrolash
-- (25165). OnEnterCombat(1): per-GUID reset, arm shadow blades
-- 45248 10s then 10s, non-triggered self-cast (C++ DoCast(me, ...),
-- default triggered=false — vaelastrasz convention) / shadow nova
-- 45329 30s then {30s,35s}, non-triggered on a random alive player
-- threat-list unit alive (C++ SelectTarget(Random, 0),
-- playerOnly=false, UnitAI.h:170; nil pick casts nothing but
-- keeps the schedule),
-- Talk(EMOTE_SHADOW_NOVA, on target) + Talk(YELL_SHADOW_NOVA,
-- even on nil pick — C++-exact) (the arm runs only while
-- !SisterDeath — the poll below switches it off on sister death;
-- the emote's C++
-- target arm is bridged — Talk(id, target) via the whisperTarget arm
-- (engine/world/creature_text.go:62; C++-exact, non-whisper rows
-- ignore the target) — both emote calls pass the target
-- (boss_eredar_twins.cpp:254, 597) /
-- confounding blow 45256 25s then {20s,25s}, non-triggered on a
-- threat-list unit alive, nil pick keeps the schedule
-- / enrage 46587 6min one-shot: Talk(YELL_ENRAGE), non-triggered
-- self-cast (C++ DoCast default triggered=false). The shadow-image
-- summon arm (3 x DoSpawnCreature NPC_SHADOW_IMAGE 25214,
-- TEMPSUMMON_CORPSE_DESPAWN 10s) is PORTED below (sacrolashShadowImage:
-- SummonCreature type 4 + EngageWithTarget; the paired cross-creature
-- AddThreat 1M leg stays out — AddThreat is self-threat only). The
-- sister-death conflagration arm 45342 is PORTED below (detection poll +
-- arm switch). OnTargetDied(3): 25% Talk(YELL_SAC_KILL), no
-- TYPEID gate (C++-exact — the C++ KilledUnit Talk has no victim
-- gate). OnDied(4): the SisterDeath-gated Talk(YELL_SAC_DEAD 4) is
-- ported via the twinDead latch (both twins' ev4 handlers live in
-- this file and fire in death order); the loot-flag arm ports as
-- RemoveFlag(79, LOOTABLE) on the sister-alive death (dynamic-flag
-- bridge live, twin_valkyr precedent). OnReset(23):
-- per-GUID reset (Enraged=false, SisterDeath=false) + cancel
-- timers; the instance sister respawn/threat relay is PORTED below
-- via creature:GetInstanceCreature (bridge live,
-- lua_creature_events.go:6680); SetBossState(DATA_EREDAR_TWINS 3,
-- NOT_STARTED) ports via creature:SetBossState (bridge live
-- lua_creature_events.go:5917).
-- OnLeaveCombat(2): cancel timers, drop per-GUID state. The
-- UpdateAI cast-state gates (boss_eredar_twins.cpp, per-arm
-- IsNonMeleeSpellCast(false) checks in the ScriptedAI UpdateAI
-- overrides — no BossAI base gate applies here): PORTED via the
-- live creature:IsCasting() bridge (lua_creature_events.go:1950,
-- mal_ganis/felmyst idiom) — sacrolash shadow blades (:291),
-- shadow nova (:245) and confounding blow (:264); alythess
-- conflagration (:586), flame sear (:608), pyrogenics (:617) and
-- blaze (:626); each defers 500ms under its own key, consuming no
-- re-arm, with the defer preceding target picks and Talks. The
-- InterruptSpell(CURRENT_GENERIC_SPELL) legs inside the
-- conflagration/shadow-nova gates are vacuous there (they fire
-- only when not casting; InterruptSpell is bridged at
-- lua_creature_events.go:987 but adds nothing in this position).
-- Ungated per C++: the sacrolash shadow-image summon (:272-287,
-- no gate), both 6min enrages (:306 sacrolash / alythess enrage
-- block, no gate), shadow-image shadow fury (no gate). Shadow-
-- image dark strike (:731) is a SKIP gate, not a defer: its 3s
-- re-arm sits OUTSIDE the gate, so a casting-blocked strike is
-- skipped and retried at the next 3s tick — ported as an
-- IsCasting condition on the cast, re-arm untouched. The old
-- "timers fire unconditionally (jeklik convention)" claim is
-- retracted for the gated arms above. The SpellHitTarget
-- HandleTouchedSpells arm (45248/45329/45256/45270 -> dark touched
-- 45347; 45342 -> flame touched 45348) is PORTED below via ev15
-- (see header top). The melee-range HandleTouchedSpells(victim, 45347)
-- arm (:306-315 AttackerStateUpdate leg) stays unmodeled — there is no
-- melee-hit-dealt hook (no Melee/DealDamage event in
-- engine/scripting/runtime.go); the swing itself is engine-driven and
-- enabled (no SetMeleeAttack(false) on this twin — C++ swings, unlike
-- Alythess). Grand Warlock Alythess (25166). OnEnterCombat(1):
-- per-GUID reset, arm conflagration 45342 45s then {30s,35s}, non-
-- triggered on a threat-list unit alive (nil pick
-- casts nothing but keeps the schedule), Talk(EMOTE_CONFLAGRATION,
-- on target) + Talk(YELL_CANFLAGRATION, even on nil pick —
-- C++-exact), then the blaze timer is re-armed
-- once at 4s (C++-exact — the conflag arm sets BlazeTimer = 4000)
-- / flame sear 46771 15s then 15s, non-triggered self-cast /
-- pyrogenics 45230 15s then 15s, triggered self-cast (C++ DoCast
-- (me, SPELL_PYROGENICS, true), C++-exact) / blaze 45235 100ms then
-- 3.8s, non-triggered DoCastVictim (nil ticks cast nothing but keep
-- the schedule, jeklik convention) / enrage 46587 6min one-shot:
-- Talk(YELL_BERSERK), non-triggered self-cast. The sister-death
-- shadow-nova arm 45329 and the empower arm are PORTED below
-- (detection poll + arm switch). OnTargetDied(3): 25% Talk(YELL_
-- ALY_KILL), no TYPEID gate. OnDied(4): sister-death-gated Talk
-- (YELL_ALY_DEAD 6) is ported via the twinDead latch; the loot-
-- flag arm ports as RemoveFlag(79, LOOTABLE) on the sister-alive
-- death (dynamic-flag bridge live). OnReset(23): per-GUID reset + cancel
-- timers; the instance sister respawn/threat relay is PORTED below
-- via creature:GetInstanceCreature (bridge live,
-- lua_creature_events.go:6680); SetBossState(DATA_EREDAR_TWINS 3,
-- NOT_STARTED) ports via creature:SetBossState (bridge live
-- lua_creature_events.go:5917). OnLeaveCombat(2):
-- cancel timers, drop per-GUID state. The SpellHitTarget arm (45235
-- -> blaze-summon 45236 self-cast on the hit target; 45342/46771 ->
-- flame touched 45348; 45329 -> dark touched 45347) is PORTED below
-- via ev15 (see header top). The 8-step intro
-- chain
-- (MoveInLineOfSight sighting + cross-creature Talk relays) stays
-- unmodeled — the trigger is sister-sighting at spawn (same-faction,
-- non-attackable unit), and Go's ev27 creature-victim scan is
-- hostile-gated (creaturemotion.go creatureHostileToCreature), so the
-- sister sighting never fires ev27. (ev5 IS a spawn hook in Go and is
-- used below for the GuidData latch — the old "no spawn hook" claim
-- was stale; the blocker is the ev27 hostile gate, not the hook.)
-- The no-victim threat handoff from the sister
-- and the sister AttackStart relays are PORTED into both enter
-- handlers (FindNearestCreature + !GetVictim gate +
-- EngageWithTarget), and the per-tick no-victim handoff itself is
-- PORTED in the sister-death poll below (it needs only the sister's
-- victim via GetInstanceCreature + GetVictim, no threat-list bridge).
-- Shadow Image (25214). OnReset(23): the NOT_
-- SELECTABLE flag arm is PORTED 2026-10-20 via the live flag bridge
-- (the old "no flag bridge" claim is retracted; C++:677-679 sets only
-- the flag + Initialize(); the cast lives in the per-tick HasAura
-- arm, C++:712). OnEnterCombat(1): arm
-- shadow fury 45270 {5s,20s} then 10s, non-triggered self-cast /
-- dark strike 45271 3s then 3s, non-triggered DoCastVictim (nil
-- ticks cast nothing but keep the schedule); a 10s visual-
-- maintenance check re-applies 45263 non-triggered if missing (the
-- C++ per-tick HasAura arm — DoCast(me,45263) takes no triggered
-- arg — approximated). The 15s KillSelf arm is PORTED in
-- shadowImageEnterCombat (creature:Kill(), C++ KillTimer 15000);
-- the old "no kill bridge" claim was stale. The
-- sacrolash-side DoSpawnCreature summon: the AttackStart(target) leg
-- is PORTED 2026-10-26 (sacrolashShadowImage, EngageWithTarget
-- bridge); the paired AddThreat(target, 1000000.0f, temp) leg stays
-- out (AddThreat is self-threat only). DB-spawned images keep their
-- own enter handler (felmyst-trail convention).
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (the twins
-- are ScriptedAI with direct instance->SetBossState writes, no
-- BossAI base involved; the DATA_EREDAR_TWINS legs ARE ported —
-- ev1 IN_PROGRESS / ev23 NOT_STARTED on both twins, ev4 DONE on
-- the second twin's death via the twinDead latch; the sister
-- AttackStart/respawn/threat relays ARE ported — enter-combat relay,
-- Reset respawn/threat relay, per-tick no-victim handoff); the
-- sister-death detection + empower + sister-phase spell arms are
-- PORTED 2026-11-05 (GetInstanceCreature bridge + 1s poll, below);
-- the alythess intro cross-Talk stays unmodeled (ev27 hostile gate,
-- above); the touched-spell chaining and the alythess blaze-summon
-- (GO 187366) arms are PORTED below via ev15 (the blaze-summon GO
-- effect itself rides the spell engine); the shadow-image spawns ARE
-- ported (SummonCreature); the cross-creature AddThreat 1M leg stays
-- out (AddThreat is self-threat only); the melee dark-touched leg
-- stays out (no melee-hit hook). The lootable-flag, no-movement and
-- image KillSelf arms are PORTED.

local SPELL_SHADOW_BLADES = 45248
local SPELL_SHADOW_NOVA = 45329
local SPELL_CONFOUNDING_BLOW = 45256
local SPELL_CONFLAGRATION = 45342
local SPELL_ENRAGE = 46587
local SPELL_PYROGENICS = 45230
local SPELL_FLAME_SEAR = 46771
local SPELL_BLAZE = 45235
local SPELL_EMPOWER = 45366
local SPELL_DARK_TOUCHED = 45347
local SPELL_FLAME_TOUCHED = 45348
local SPELL_DARK_FLAME = 45345
local SPELL_BLAZE_SUMMON = 45236

-- UnitDefines.h:149; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59) is
-- bridged (engine/world/lua_creature_events.go:968).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local SPELL_IMAGE_VISUAL = 45263
local SPELL_SHADOW_FURY = 45270
local SPELL_DARK_STRIKE = 45271

local YELL_SAC_KILL = 8
local YELL_SAC_DEAD = 4
local EMOTE_SHADOW_NOVA = 5
local YELL_SHADOW_NOVA = 9
local YELL_ENRAGE = 6
local EMOTE_CONFLAGRATION = 4
local YELL_CANFLAGRATION = 8
local YELL_ALY_KILL = 5
local YELL_ALY_DEAD = 6
local YELL_BERSERK = 9
local YELL_SISTER_ALYTHESS_DEAD = 7
local YELL_SISTER_SACROLASH_DEAD = 7

-- sunwell_plateau.h SWPDataTypes: DATA_ALYTHESS = 10, DATA_SACROLASH = 11
-- (DATA_KALECGOS 0, BRUTALLUS 1, FELMYST 2, EREDAR_TWINS 3, MURU 4,
-- KILJAEDEN 5, then KALECGOS_DRAGON 6, HUMAN 7, SATHROVARR 8,
-- MADRIGOSA 9, ALYTHESS 10, SACROLASH 11).
local DATA_ALYTHESS = 10
local DATA_SACROLASH = 11
local DATA_EREDAR_TWINS = 3 -- sunwell_plateau.h:34

local timers = {}
local sacrolashState = {}
local alythessState = {}

-- Sister-death latch for the JustDied YELL_SAC_DEAD / YELL_ALY_DEAD arms
-- (boss_eredar_twins.cpp:150-163 sacrolash, :431-441 alythess). The C++
-- SisterDeath bool is set on the SURVIVING twin's AI when its UpdateAI
-- poll observes the sister dead, and the DYING twin's JustDied reads its
-- own latch — so the yell fires exactly when the sister died before this
-- twin. Both twins' ev4 handlers live in this file and fire in death
-- order, so the latch is module state set in ev4: no cross-creature
-- lookup is needed for the JustDied yell (the UpdateAI empower /
-- YELL_SISTER_*_DEAD detection arms are ported separately below via
-- the GetInstanceCreature bridge + 1s poll). Go fires ev23
-- before ev4 on the death path (fireCreatureDied), so the reset handlers
-- below clear the latch only when the creature is alive — C++ Reset
-- never runs on the death path (the spark_overgrind IsDead-gate
-- precedent).
local twinDead = { sacrolash = false, alythess = false }

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79);
-- UNIT_DYNFLAG_LOOTABLE (SharedDefines.h:3128) — the dynamic-flag
-- bridge is live (twin_valkyr precedent); the old "lootable-flag arm
-- has no flag bridge" claims are retracted 2026-10-21.
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function cancelTimerKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key])
        per[key] = nil
    end
end


-- C++ SelectTarget(Random, 0) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible. Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345).
-- Dead victims go offline via ShouldBeOffline / _IsTargetAcceptable
-- (Creature.cpp:2508, UNIT_STATE_DIED) and SelectTargetList skips
-- offline refs (UnitAI.h), so the pick filters dead entries.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    local alive = {}
    for _, t in ipairs(targets) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

-- C++ HandleTouchedSpells, shared by both twins (boss_eredar_twins.cpp
-- :184 sacrolash / :465 alythess): a hit unit without dark flame
-- upgrades dark-touched <-> flame-touched into dark flame, else takes
-- the single touched aura. RemoveAura(spell) is the
-- RemoveAurasDueToSpell twin (Aura::Remove() default); the target
-- self-casts triggered, as in C++ — player targets take the 2-arg
-- CastSpell (the player funnel is triggered-only), creature targets
-- the 3rd-arg triggered form.
local function handleTouchedSpells(target, touchedType)
    if touchedType == SPELL_FLAME_TOUCHED then
        if not target:HasAura(SPELL_DARK_FLAME) then
            if target:HasAura(SPELL_DARK_TOUCHED) then
                target:RemoveAura(SPELL_DARK_TOUCHED)
                target:CastSpell(target, SPELL_DARK_FLAME, true)
            else
                target:CastSpell(target, SPELL_FLAME_TOUCHED, true)
            end
        end
    else
        if not target:HasAura(SPELL_DARK_FLAME) then
            if target:HasAura(SPELL_FLAME_TOUCHED) then
                target:RemoveAura(SPELL_FLAME_TOUCHED)
                target:CastSpell(target, SPELL_DARK_FLAME, true)
            else
                target:CastSpell(target, SPELL_DARK_TOUCHED, true)
            end
        end
    end
end

-- ================= Lady Sacrolash (25165) =================

-- C++ EVENT shadow blades: DoCastSelf(45248); repeat 10s.
local function sacrolashShadowBlades(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowblades", 500, function()
            sacrolashShadowBlades(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SHADOW_BLADES)
    schedule(guid, "shadowblades", 10000, function()
        sacrolashShadowBlades(creature, guid)
    end)
end

-- C++ EVENT shadow nova (the !SisterDeath branch): DoCast(random
-- target, 45329) + Talk(EMOTE_SHADOW_NOVA) + Talk(YELL_SHADOW_NOVA);
-- repeat {30s,35s}. The arm stops when the sister-death poll latches
-- (the pending event is cancelled and the latch guard below drops any
-- in-flight tick); C++ fires YELL_SHADOW_NOVA even when the target
-- pick is nil — Talk placement matches.
local function sacrolashShadowNova(creature, guid)
    local st = sacrolashState[guid]
    if st and st.sisterDeath then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "shadownova", 500, function()
            sacrolashShadowNova(creature, guid)
        end)
        return
    end
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_NOVA)
        creature:Talk(EMOTE_SHADOW_NOVA, target)
    end
    creature:Talk(YELL_SHADOW_NOVA)
    -- C++ 30000 + rand32()%5000 is exclusive-top [30000,34999].
    schedule(guid, "shadownova", math.random(30000, 34999), function()
        sacrolashShadowNova(creature, guid)
    end)
end

-- C++ EVENT confounding blow: DoCast(random target, 45256);
-- repeat {20s,25s}.
local function sacrolashConfoundingBlow(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "confoundingblow", 500, function()
            sacrolashConfoundingBlow(creature, guid)
        end)
        return
    end
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFOUNDING_BLOW)
    end
    -- C++ 20000 + rand32()%5000 is exclusive-top [20000,24999].
    schedule(guid, "confoundingblow", math.random(20000, 24999), function()
        sacrolashConfoundingBlow(creature, guid)
    end)
end

-- C++ EVENT enrage: InterruptSpell(CURRENT_GENERIC_SPELL) (:300) +
-- Talk(YELL_ENRAGE) + DoCastSelf(46587); 6min one-shot, Enraged
-- once-guard (C++-exact).
local function sacrolashEnrage(creature, guid)
    local st = sacrolashState[guid]
    if st and not st.enraged then
        st.enraged = true
        creature:InterruptSpell(1) -- CURRENT_GENERIC_SPELL (boss_eredar_twins.cpp:300)
        creature:Talk(YELL_ENRAGE)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

-- C++ EVENT conflagration (the SisterDeath branch, :224-235):
-- InterruptSpell(CURRENT_GENERIC_SPELL) + DoCast(random target,
-- 45342); repeat {30s,35s}; no Talk on this twin's arm. Armed at
-- 30000 on the sister-death detection — ConflagrationTimer only
-- decrements while SisterDeath is true, so it still holds its
-- Initialize value then.
local function sacrolashConflagration(creature, guid)
    local st = sacrolashState[guid]
    if st and st.sisterDeath then
        if creature:IsCasting() then
            schedule(guid, "conflagration", 500, function()
                sacrolashConflagration(creature, guid)
            end)
            return
        end
        creature:InterruptSpell(1) -- CURRENT_GENERIC_SPELL (:234)
        local target = randomThreatTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_CONFLAGRATION)
        end
        -- C++ 30000 + rand32()%5000 is exclusive-top [30000,34999].
        schedule(guid, "conflagration", math.random(30000, 34999), function()
            sacrolashConflagration(creature, guid)
        end)
    end
end

-- C++ UpdateAI sister-death detection (:212-221): while !SisterDeath,
-- each AI tick reads instance->GetCreature(DATA_ALYTHESS) and latches
-- on isDead: Talk(YELL_SISTER_ALYTHESS_DEAD 7) + DoCast(me,
-- SPELL_EMPOWER 45366) + InterruptSpell(CURRENT_GENERIC_SPELL), then
-- the shadow-nova arm stops and the conflagration arm starts. The Go
-- poll runs at 1s while in combat (sironas convention); C++ polls
-- every AI tick including out of combat, but the sister can only die
-- in combat, so the in-combat poll matches in practice.
-- GetInstanceCreature is the tick-221 bridge; the twins self-latch
-- their DATA ids on spawn (ev5, illidari-council idiom).
local function sacrolashSisterDeathPoll(creature, guid)
    local st = sacrolashState[guid]
    if not st or st.sisterDeath then
        return
    end
    local sister = creature:GetInstanceCreature(DATA_ALYTHESS)
    if sister ~= nil and sister:IsDead() then
        st.sisterDeath = true
        creature:Talk(YELL_SISTER_ALYTHESS_DEAD)
        creature:CastSpell(creature, SPELL_EMPOWER)
        creature:InterruptSpell(1) -- CURRENT_GENERIC_SPELL (:220)
        cancelTimerKey(guid, "shadownova")
        schedule(guid, "conflagration", 30000, function()
            sacrolashConflagration(creature, guid)
        end)
        return
    end
    schedule(guid, "sisterdeath", 1000, function()
        sacrolashSisterDeathPoll(creature, guid)
    end)
end

-- C++ instance OnCreatureCreate latching (InstanceScript::AddObject):
-- the twin's GUID goes in the instance store under DATA_SACROLASH so
-- the sister's poll can resolve it via GetInstanceCreature.
local function sacrolashSpawn(event, creature)
    creature:SetGuidData(DATA_SACROLASH)
end

-- C++ JustEngagedWith + Initialize: per-GUID reset, arm the
-- five bridgeable timers + the sister-death poll.
-- C++ ShadowimageTimer (boss_eredar_twins.cpp:272-287): 20s
-- init/repeat — 3x fresh random threat target each (SelectTarget
-- Random,0, playerOnly=FALSE) + DoSpawnCreature(NPC_SHADOW_IMAGE
-- 25214, boss position, TEMPSUMMON_CORPSE_DESPAWN 10s) +
-- AttackStart(target) when both exist; the 1M cross-creature
-- AddThreat leg stays out (AddThreat is self-threat only). The
-- summoned images run their own enter handler (DB-spawned
-- images share it); EngageWithTarget resolves creature targets too
-- (session or motion lookup), so non-player picks engage as in C++.
local function sacrolashShadowImage(creature, guid)
    for _ = 1, 3 do
        local target = randomThreatTarget(creature)
        local image = creature:SummonCreature(25214,
            creature:GetX(), creature:GetY(), creature:GetZ(),
            creature:GetO(), 4, 10000)
        if image ~= nil and target ~= nil then
            image:EngageWithTarget(target)
        end
    end
    schedule(guid, "shadowimage", 20000, function()
        sacrolashShadowImage(creature, guid)
    end)
end

local function sacrolashEnterCombat(event, creature, target)
    creature:DoZoneInCombat() -- boss_eredar_twins.cpp:136 (JustEngagedWith arm 1; bridge live 2026-10-23)
    -- C++ JustEngagedWith (:140-144): sister Alythess (25166; instance
    -- GuidData in C++, the FindNearestCreature idiom here — the entry
    -- is fight-unique, so the scan is the instance lookup) engages
    -- the engager when alive and victimless. The back-relay fires
    -- Alythess's own enter handler, whose !GetVictim gate blocks the
    -- return leg, so no loop. SetBossState(DATA_EREDAR_TWINS 3,
    -- IN_PROGRESS) ports below (boss_eredar_twins.cpp:142;
    -- creature:SetBossState bridge live lua_creature_events.go:5917).
    local victim = target or creature:GetVictim()
    if victim ~= nil then
        local sister = creature:FindNearestCreature(25166, 100)
        if sister ~= nil and sister:IsAlive() and sister:GetVictim() == nil then
            sister:EngageWithTarget(victim)
        end
    end
    creature:SetBossState(DATA_EREDAR_TWINS, 1)
    -- No SetMeleeAttack(false) here: C++ swings in melee range
    -- (boss_eredar_twins.cpp:306-315: isAttackReady + IsWithinMeleeRange
    -- + AttackerStateUpdate — the julianne precedent, a direct UpdateAI
    -- swing, not a DoMeleeAttackIfReady omission), so the engine-driven
    -- swing stays enabled. Only the melee dark-touched application stays
    -- out (no melee-hit hook; see header).
    local guid = creature:GetGUIDLow()
    sacrolashState[guid] = { enraged = false, sisterDeath = false }
    schedule(guid, "shadowblades", 10000, function()
        sacrolashShadowBlades(creature, guid)
    end)
    schedule(guid, "shadownova", 30000, function()
        sacrolashShadowNova(creature, guid)
    end)
    schedule(guid, "confoundingblow", 25000, function()
        sacrolashConfoundingBlow(creature, guid)
    end)
    schedule(guid, "shadowimage", 20000, function()
        sacrolashShadowImage(creature, guid)
    end)
    schedule(guid, "enrage", 360000, function()
        sacrolashEnrage(creature, guid)
    end)
    schedule(guid, "sisterdeath", 1000, function()
        sacrolashSisterDeathPoll(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(YELL_SAC_KILL), no victim gate.
local function sacrolashTargetDied(event, creature)
    if math.random(1, 4) == 1 then
        creature:Talk(YELL_SAC_KILL)
    end
end

-- C++ JustDied (:151-163): if (SisterDeath) Talk(YELL_SAC_DEAD 4) +
-- SetBossState(DATA_EREDAR_TWINS 3, DONE) (:156-158, Talk first) —
-- the latch is twinDead.alythess (sister died first); the
-- lootable-flag arm ports as the else-branch RemoveFlag(79)
-- (C++-verbatim: sister-alive death clears LOOTABLE on me — Go's
-- lethal path auto-set it, twin_valkyr precedent).
local function sacrolashDied(event, creature)
    twinDead.sacrolash = true
    if twinDead.alythess then
        creature:Talk(YELL_SAC_DEAD)
        creature:SetBossState(DATA_EREDAR_TWINS, 3)
    else
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
    end
end

local function sacrolashLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    sacrolashState[guid] = nil
end

local function sacrolashReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    sacrolashState[guid] = nil
    if not creature:IsDead() then
        twinDead.sacrolash = false
        twinDead.alythess = false
        -- C++ Reset (boss_eredar_twins.cpp:114-132): direct
        -- instance->SetBossState(DATA_EREDAR_TWINS 3, NOT_STARTED)
        -- (:131), no gate in C++; Go fires ev23 on the death path
        -- too, hence the alive gate (C++ Reset never runs on death).
        -- Sister leg (:118-125) ports first, in C++ order: dead
        -- sister respawns, live sister's victim is added to my threat
        -- (GetInstanceCreature bridge live; sister GUIDs latched at
        -- spawn via SetGuidData).
        local sister = creature:GetInstanceCreature(DATA_ALYTHESS)
        if sister ~= nil then
            if sister:IsDead() then
                sister:Respawn()
            else
                local victim = sister:GetVictim()
                if victim ~= nil then
                    creature:AddThreat(victim, 0)
                end
            end
        end
        creature:SetBossState(DATA_EREDAR_TWINS, 0)
    end
end

-- C++ boss_sacrolashAI::SpellHitTarget (:164-182): shadow blades /
-- shadow nova / confounding blow / shadow fury hits take dark touched,
-- conflagration hits take flame touched. Ev15 fires once per actual
-- hit unit (see header top); the caster herself is hitTargets[0] of
-- her own self-cast AoEs, so she is skipped — C++ never fires
-- SpellHitTarget on the caster. (Shadow fury never fires here — the
-- image's casts fire on the image's bindings — but the case mirrors
-- the C++ switch.)
local function sacrolashSpellHitTarget(event, creature, target, spellId)
    if target == nil or target:GetGUIDLow() == creature:GetGUIDLow() then
        return
    end
    if spellId == SPELL_SHADOW_BLADES or spellId == SPELL_SHADOW_NOVA or
        spellId == SPELL_CONFOUNDING_BLOW or spellId == SPELL_SHADOW_FURY then
        handleTouchedSpells(target, SPELL_DARK_TOUCHED)
    elseif spellId == SPELL_CONFLAGRATION then
        handleTouchedSpells(target, SPELL_FLAME_TOUCHED)
    end
end

-- ================= Grand Warlock Alythess (25166) =================

-- C++ EVENT blaze: DoCastVictim(45235); repeat 3.8s. The
-- conflagration arm re-arms this once at 4s (C++-exact).
local function alythessBlaze(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "blaze", 500, function()
            alythessBlaze(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_BLAZE)
    schedule(guid, "blaze", 3800, function()
        alythessBlaze(creature, guid)
    end)
end

-- C++ EVENT conflagration (the !SisterDeath branch): DoCast(random
-- target, 45342) + Talk(EMOTE_CONFLAGRATION) +
-- Talk(YELL_CANFLAGRATION), BlazeTimer = 4000; repeat {30s,35s}.
-- The arm stops when the sister-death poll latches (the pending event
-- is cancelled and the latch guard below drops any in-flight tick);
-- C++ fires YELL_CANFLAGRATION and the blaze re-arm even when the
-- target pick is nil — placement matches.
local function alythessConflagration(creature, guid)
    local st = alythessState[guid]
    if st and st.sisterDeath then
        return
    end
    if creature:IsCasting() then
        schedule(guid, "conflagration", 500, function()
            alythessConflagration(creature, guid)
        end)
        return
    end
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_CONFLAGRATION)
        creature:Talk(EMOTE_CONFLAGRATION, target)
    end
    creature:Talk(YELL_CANFLAGRATION)
    schedule(guid, "blaze", 4000, function()
        alythessBlaze(creature, guid)
    end)
    -- C++ 30000 + rand32()%5000 is exclusive-top [30000,34999].
    schedule(guid, "conflagration", math.random(30000, 34999), function()
        alythessConflagration(creature, guid)
    end)
end

-- C++ EVENT flame sear: DoCastSelf(46771); repeat 15s.
local function alythessFlameSear(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flamesear", 500, function()
            alythessFlameSear(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_FLAME_SEAR)
    schedule(guid, "flamesear", 15000, function()
        alythessFlameSear(creature, guid)
    end)
end

-- C++ EVENT pyrogenics: triggered DoCastSelf(45230); repeat 15s.
local function alythessPyrogenics(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "pyrogenics", 500, function()
            alythessPyrogenics(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_PYROGENICS, true)
    schedule(guid, "pyrogenics", 15000, function()
        alythessPyrogenics(creature, guid)
    end)
end

-- C++ EVENT enrage: InterruptSpell(CURRENT_GENERIC_SPELL) (:635) +
-- Talk(YELL_BERSERK) + DoCastSelf(46587); 6min one-shot, Enraged
-- once-guard (C++-exact).
local function alythessEnrage(creature, guid)
    local st = alythessState[guid]
    if st and not st.enraged then
        st.enraged = true
        creature:InterruptSpell(1) -- CURRENT_GENERIC_SPELL (boss_eredar_twins.cpp:635)
        creature:Talk(YELL_BERSERK)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end
end

-- C++ EVENT shadow nova (the SisterDeath branch, :570-580):
-- DoCast(random target, 45329); repeat {30s,35s}; no Talk on this
-- twin's arm. Armed at 40000 on the sister-death detection —
-- ShadownovaTimer only decrements while SisterDeath is true, so it
-- still holds its Initialize value then.
local function alythessShadowNova(creature, guid)
    local st = alythessState[guid]
    if st and st.sisterDeath then
        if creature:IsCasting() then
            schedule(guid, "shadownova", 500, function()
                alythessShadowNova(creature, guid)
            end)
            return
        end
        local target = randomThreatTarget(creature)
        if target then
            creature:CastSpell(target, SPELL_SHADOW_NOVA)
        end
        -- C++ 30000 + rand32()%5000 is exclusive-top [30000,34999].
        schedule(guid, "shadownova", math.random(30000, 34999), function()
            alythessShadowNova(creature, guid)
        end)
    end
end

-- C++ UpdateAI sister-death detection (:545-553): while !SisterDeath,
-- each AI tick reads instance->GetCreature(DATA_SACROLASH) and latches
-- on isDead: Talk(YELL_SISTER_SACROLASH_DEAD 7) + DoCast(me,
-- SPELL_EMPOWER 45366) + InterruptSpell(CURRENT_GENERIC_SPELL), then
-- the conflagration arm stops and the shadow-nova arm starts. The Go
-- poll runs at 1s while in combat (sironas convention); C++ polls
-- every AI tick including out of combat, but the sister can only die
-- in combat, so the in-combat poll matches in practice.
local function alythessSisterDeathPoll(creature, guid)
    local st = alythessState[guid]
    if not st or st.sisterDeath then
        return
    end
    -- C++ UpdateAI no-victim handoff (:555-565): while the sister lives
    -- and holds a victim I have none of, take her victim (threat 0 +
    -- DoStartNoMovement + Attack — EngageWithTarget; no movement, as
    -- SetCombatMovement(false)). Runs on the 1s poll; after sister
    -- death C++ no-ops (sister dead), matching the poll stopping on
    -- the latch above.
    if creature:GetVictim() == nil then
        local sister = creature:GetInstanceCreature(DATA_SACROLASH)
        if sister ~= nil and not sister:IsDead() then
            local victim = sister:GetVictim()
            if victim ~= nil then
                creature:AddThreat(victim, 0)
                creature:EngageWithTarget(victim)
            end
        end
    end
    local sister = creature:GetInstanceCreature(DATA_SACROLASH)
    if sister ~= nil and sister:IsDead() then
        st.sisterDeath = true
        creature:Talk(YELL_SISTER_SACROLASH_DEAD)
        creature:CastSpell(creature, SPELL_EMPOWER)
        creature:InterruptSpell(1) -- CURRENT_GENERIC_SPELL (:552)
        cancelTimerKey(guid, "conflagration")
        schedule(guid, "shadownova", 40000, function()
            alythessShadowNova(creature, guid)
        end)
        return
    end
    schedule(guid, "sisterdeath", 1000, function()
        alythessSisterDeathPoll(creature, guid)
    end)
end

-- C++ instance OnCreatureCreate latching (InstanceScript::AddObject):
-- the twin's GUID goes in the instance store under DATA_ALYTHESS so
-- the sister's poll can resolve it via GetInstanceCreature.
local function alythessSpawn(event, creature)
    creature:SetGuidData(DATA_ALYTHESS)
    creature:SetCombatMovement(false) -- ScriptedAI ctor arm, boss_eredar_twins.cpp:335 (ranged AI stands and casts)
end

-- C++ JustEngagedWith + Initialize: per-GUID reset, arm the
-- five bridgeable timers + the sister-death poll.
local function alythessEnterCombat(event, creature, target)
    creature:DoZoneInCombat() -- boss_eredar_twins.cpp:392 (JustEngagedWith arm 1; bridge live 2026-10-23)
    -- C++ JustEngagedWith (:396-400): sister Sacrolash (25165; instance
    -- GuidData in C++, the FindNearestCreature idiom here — the entry
    -- is fight-unique) engages the engager when alive and
    -- victimless; the back-relay is gated by Sacrolash's own
    -- !GetVictim check. SetBossState(DATA_EREDAR_TWINS 3,
    -- IN_PROGRESS) ports below (boss_eredar_twins.cpp:398;
    -- creature:SetBossState bridge live lua_creature_events.go:5917).
    local victim = target or creature:GetVictim()
    if victim ~= nil then
        local sister = creature:FindNearestCreature(25165, 100)
        if sister ~= nil and sister:IsAlive() and sister:GetVictim() == nil then
            sister:EngageWithTarget(victim)
        end
    end
    creature:SetBossState(DATA_EREDAR_TWINS, 1)
    creature:SetMeleeAttack(false) -- boss_eredar_twins.cpp:535: her UpdateAI has no melee arm anywhere (cast-only in C++)
    local guid = creature:GetGUIDLow()
    alythessState[guid] = { enraged = false, sisterDeath = false }
    schedule(guid, "conflagration", 45000, function()
        alythessConflagration(creature, guid)
    end)
    schedule(guid, "flamesear", 15000, function()
        alythessFlameSear(creature, guid)
    end)
    schedule(guid, "pyrogenics", 15000, function()
        alythessPyrogenics(creature, guid)
    end)
    schedule(guid, "blaze", 100, function()
        alythessBlaze(creature, guid)
    end)
    schedule(guid, "enrage", 360000, function()
        alythessEnrage(creature, guid)
    end)
    schedule(guid, "sisterdeath", 1000, function()
        alythessSisterDeathPoll(creature, guid)
    end)
end

-- C++ KilledUnit: 25% Talk(YELL_ALY_KILL), no victim gate.
local function alythessTargetDied(event, creature)
    if math.random(1, 4) == 1 then
        creature:Talk(YELL_ALY_KILL)
    end
end

-- C++ JustDied (:433-441): if (SisterDeath) Talk(YELL_ALY_DEAD 6) +
-- SetBossState(DATA_EREDAR_TWINS 3, DONE) (:436-438, Talk first) —
-- the latch is twinDead.sacrolash (sister died first); the
-- lootable-flag arm ports as the else-branch RemoveFlag(79)
-- (C++-verbatim, see sacrolashDied above).
local function alythessDied(event, creature)
    twinDead.alythess = true
    if twinDead.sacrolash then
        creature:Talk(YELL_ALY_DEAD)
        creature:SetBossState(DATA_EREDAR_TWINS, 3)
    else
        creature:RemoveFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
    end
end

local function alythessLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    alythessState[guid] = nil
end

local function alythessReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    alythessState[guid] = nil
    if not creature:IsDead() then
        twinDead.sacrolash = false
        twinDead.alythess = false
        -- C++ Reset (boss_eredar_twins.cpp:370-388): direct
        -- instance->SetBossState(DATA_EREDAR_TWINS 3, NOT_STARTED)
        -- (:387), no gate in C++; Go fires ev23 on the death path
        -- too, hence the alive gate (C++ Reset never runs on death).
        -- Sister leg (:374-381) ports first, in C++ order: dead
        -- sister respawns, live sister's victim is added to my threat
        -- (GetInstanceCreature bridge live; sister GUIDs latched at
        -- spawn via SetGuidData).
        local sister = creature:GetInstanceCreature(DATA_SACROLASH)
        if sister ~= nil then
            if sister:IsDead() then
                sister:Respawn()
            else
                local victim = sister:GetVictim()
                if victim ~= nil then
                    creature:AddThreat(victim, 0)
                end
            end
        end
        creature:SetBossState(DATA_EREDAR_TWINS, 0)
    end
end

-- C++ boss_alythessAI::SpellHitTarget (:444-462): blaze hits make the
-- hit unit self-cast blaze-summon 45236 (triggered); conflagration /
-- flame sear hits take flame touched, shadow nova hits take dark
-- touched. Caster-self skip as in sacrolashSpellHitTarget.
local function alythessSpellHitTarget(event, creature, target, spellId)
    if target == nil or target:GetGUIDLow() == creature:GetGUIDLow() then
        return
    end
    if spellId == SPELL_BLAZE then
        target:CastSpell(target, SPELL_BLAZE_SUMMON, true)
    elseif spellId == SPELL_CONFLAGRATION or spellId == SPELL_FLAME_SEAR then
        handleTouchedSpells(target, SPELL_FLAME_TOUCHED)
    elseif spellId == SPELL_SHADOW_NOVA then
        handleTouchedSpells(target, SPELL_DARK_TOUCHED)
    end
end

-- ================= Shadow Image (25214) =================

-- C++ EVENT shadow fury: DoCastSelf(45270); first {5s,20s},
-- then 10s.
local function shadowImageFury(creature, guid)
    creature:CastSpell(creature, SPELL_SHADOW_FURY)
    schedule(guid, "shadowfury", 10000, function()
        shadowImageFury(creature, guid)
    end)
end

-- C++ EVENT dark strike: DoCastVictim(45271) only when within
-- melee range of the victim (creature:IsWithinMeleeRange) and
-- not casting (boss_eredar_twins.cpp:731 IsNonMeleeSpellCast
-- gate); the 3s re-arm sits OUTSIDE the gate, so a gated strike
-- is skipped, not deferred — the IsCasting condition below, with
-- the re-arm untouched, is C++-exact.
local function shadowImageDarkStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim and not creature:IsCasting() and creature:IsWithinMeleeRange(victim) then
        creature:CastSpell(nil, SPELL_DARK_STRIKE)
    end
    schedule(guid, "darkstrike", 3000, function()
        shadowImageDarkStrike(creature, guid)
    end)
end

-- C++ per-tick image-visual maintenance arm (approximated at
-- 10s): non-triggered self-cast 45263 if the aura is missing.
local function shadowImageVisualCheck(creature, guid)
    if not creature:HasAura(SPELL_IMAGE_VISUAL) then
        creature:CastSpell(creature, SPELL_IMAGE_VISUAL)
    end
    schedule(guid, "visualcheck", 10000, function()
        shadowImageVisualCheck(creature, guid)
    end)
end

-- C++ JustEngagedWith (empty in C++) + Initialize: arm the
-- combat timers, including the 15s KillSelf arm (ported via
-- creature:Kill(); Reset re-arms it, and the evade cycle re-arms
-- via LeaveCombat cancel + EnterCombat).
local function shadowImageEnterCombat(event, creature)
    local guid = creature:GetGUIDLow()
    -- No white swing in C++: its UpdateAI (:709-742) has no
    -- DoMeleeAttackIfReady / AttackerStateUpdate arm — dark strike is a
    -- spell cast, not a swing — so suppress the engine-driven swing.
    creature:SetMeleeAttack(false)
    -- C++ 5000 + rand32()%15000 is exclusive-top [5000,19999].
    schedule(guid, "shadowfury", math.random(5000, 19999), function()
        shadowImageFury(creature, guid)
    end)
    schedule(guid, "darkstrike", 3000, function()
        shadowImageDarkStrike(creature, guid)
    end)
    schedule(guid, "visualcheck", 10000, function()
        shadowImageVisualCheck(creature, guid)
    end)
    schedule(guid, "kill", 15000, function()
        creature:Kill()
    end)
end

local function shadowImageLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

-- C++ npc_shadow_imageAI::Reset (boss_eredar_twins.cpp:678-680):
-- SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE), then
-- Initialize(); the visual is the per-tick maintenance arm above.
-- No RemoveFlag arm exists in the C++ file, so Reset re-asserts on
-- evade (ported 2026-10-20); skip on the death path — C++ Reset
-- never runs there (Eluna On_Reset fires ahead of OnDied).
local function shadowImageReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    if not creature:IsDead() then
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    end
end

-- C++ npc_shadow_imageAI::SpellHitTarget (:685-706): shadow fury /
-- dark strike hits take the dark-touched leg only (flame-touched
-- upgrades to dark flame, else dark touched). Caster-self skip as in
-- sacrolashSpellHitTarget.
local function shadowImageSpellHitTarget(event, creature, target, spellId)
    if target == nil or target:GetGUIDLow() == creature:GetGUIDLow() then
        return
    end
    if spellId == SPELL_SHADOW_FURY or spellId == SPELL_DARK_STRIKE then
        handleTouchedSpells(target, SPELL_DARK_TOUCHED)
    end
end

RegisterCreatureEvent(25165, 1, sacrolashEnterCombat)
RegisterCreatureEvent(25165, 2, sacrolashLeaveCombat)
RegisterCreatureEvent(25165, 3, sacrolashTargetDied)
RegisterCreatureEvent(25165, 4, sacrolashDied)
RegisterCreatureEvent(25165, 23, sacrolashReset)
RegisterCreatureEvent(25165, 5, sacrolashSpawn)
RegisterCreatureEvent(25165, 15, sacrolashSpellHitTarget)

RegisterCreatureEvent(25166, 1, alythessEnterCombat)
RegisterCreatureEvent(25166, 2, alythessLeaveCombat)
RegisterCreatureEvent(25166, 3, alythessTargetDied)
RegisterCreatureEvent(25166, 4, alythessDied)
RegisterCreatureEvent(25166, 23, alythessReset)
RegisterCreatureEvent(25166, 5, alythessSpawn)
RegisterCreatureEvent(25166, 15, alythessSpellHitTarget)

RegisterCreatureEvent(25214, 1, shadowImageEnterCombat)
RegisterCreatureEvent(25214, 2, shadowImageLeaveCombat)
RegisterCreatureEvent(25214, 23, shadowImageReset)
RegisterCreatureEvent(25214, 15, shadowImageSpellHitTarget)
