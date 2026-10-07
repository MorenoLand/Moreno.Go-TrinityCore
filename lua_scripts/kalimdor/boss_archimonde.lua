-- Battle for Mount Hyjal: Archimonde — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/
-- boss_archimonde.cpp (582 lines; class boss_archimonde : public
-- CreatureScript { struct boss_archimondeAI : public BossAI(creature,
-- DATA_ARCHIMONDE) } — note: BossAI, not hyjal_trashAI, but GetAI still
-- goes through GetHyjalAI<boss_archimondeAI>; npc_ancient_wisp /
-- npc_doomfire / npc_doomfire_targetting : public CreatureScript (AI :
-- ScriptedAI via GetHyjalAI); spell_archimonde_drain_world_tree_dummy
-- SpellScriptLoader; AddSC_boss_archimonde at end registers all five;
-- kalimdor loader decl 27 / call 140 per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_archimonde", "npc_ancient_wisp", "npc_doomfire",
-- "npc_doomfire_targetting" and
-- "spell_archimonde_drain_world_tree_dummy" (loader lines only
-- otherwise).
-- Entry: hyjal.h:82 HYCreaturesIds names ARCHIMONDE = 17968 under the
-- "Bosses summoned after every 8 waves" comment AND
-- instance_hyjal.cpp:128 cases it in OnCreatureCreate (Archimonde GUID
-- capture, :158 the DATA_ARCHIMONDE GetGuidData leg) — the name-to-entry
-- tie is C++-verified (ramstein strength); the creature_template
-- ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 5 OnSpawn, 9 OnDamageTaken, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - OnSpawn (event 5, storm_cloud convention): ACTION_CHANNEL_WORLD_TREE
--   (C++ InitializeAI -> DoAction at spawn) — DoCastAOE(39140, triggered)
--   resolves to a triggered self-cast (illidan precedent).
-- - Engage (C++ JustEngagedWith): Talk SAY_ONAGGRO (1) + arm each
--   bridgeable timer at its C++ ScheduleEvent cooldown.
-- - Fear 31970: DoCastAOE resolves to self-cast (kazrogal/illidan
--   precedent), non-triggered, init 42s -> 42s.
-- - Air Burst 32014: Talk SAY_AIR_BURST (3) + cast on
--   SelectTargetMethod::Random, 1 (C++ comment "not on tank" —
--   approximated as a random alive player excluding the current victim,
--   azgalor victim-exclusion convention, no threat-list bridge),
--   non-triggered, init 30s -> {25s,40s}.
-- - Grip of the Legion 31972: SelectTargetMethod::Random, 0 with no
--   range cap (unbounded alive player in the instance — nefarian
--   randomAlivePlayer convention) EXCLUDING the tank (C++-exact:
--   withTank=false), non-triggered, init {5s,25s} -> {5s,25s}.
-- - Finger of Death 31984: init 15s. C++ checks
--   SelectTarget(Random, 0, 5.0f): no target (tank excluded) within 5
--   yards -> cast on SelectTarget(Random, 0) (tank excluded; a second,
--   independent selection — may be nil, DoCast nil no-op), re-arm 1s;
--   target in melee range -> re-arm 5s (no tank-revictiming leg in the
--   actual code — the class comment's aggro-swap paragraph is aspirational).
-- - Hand of Death 35354 (the 10-minute raid wiper): DoCastAOE resolves
--   to self-cast, non-triggered, init 10min -> 2s (C++-exact).
-- - Soul charges: C++ KilledUnit casts SPELL_SOUL_CHARGE_RED/YELLOW/GREEN
--   (32045/32051/32052) triggered on the boss from the dead victim by
--   class bucket (priest/paladin/warlock -> red; mage/rogue/warrior ->
--   yellow; druid/shaman/hunter -> green) and schedules
--   EVENT_UNLEASH_SOUL_CHARGE {2s,30s}; the unleash handler picks
--   urand(0,2) (0 -> red 32053, 1 -> yellow 32054, 2 -> green 32057) and,
--   if the matching aura is stacked, removes one stack, DoCastVictim's
--   the unleash spell, decrements the count, and re-arms {2s,30s} (no
--   re-arm when the aura is absent). The victim->CastSpell(me, triggered)
--   delivery and aura-stack visuals have no bridge, so the stacks are
--   tracked as Lua counters (aura-stack fidelity is abstracted; the
--   InterruptNonMeleeSpells arm has no bridge — maiden precedent). The
--   engage-scheduled {2s,30s} unleash is a no-op in C++ until a kill
--   lands (no re-arm on the absent-aura path), so unleash scheduling comes
--   only from kills — C++-equivalent.
-- - KilledUnit (event 3): Talk SAY_SLAY (4) unconditionally (C++ talks
--   before the TYPEID gate), then the charge leg above for players
--   (victim:GetObjectType() == "Player" gate — terestian_illhoof
--   convention).
-- - DamageTaken (event 9, wired — pre-damage, aku_mai convention):
--   HealthBelowPctDamaged(10, damage): if not yet enraged -> Talk
--   SAY_ENRAGE (5) + enraged flag (C++ MotionMaster Clear + MoveIdle has
--   no bridge); if not yet protected -> triggered self-cast
--   SPELL_PROTECTION_OF_ELUNE 38528 (DoCastAOE(triggered) resolves to
--   self-cast, triggered aku_mai convention) + protected flag. The
--   EVENT_SUMMON_WHISP leg scheduled here needs DoSpawnCreature — no
--   bridge, not armed (see below).
-- - Death (C++ JustDied): Talk SAY_ONDEATH (6); the
--   instance->SetData(DATA_ARCHIMONDE, DONE) leg and BossAI::_JustDied
--   are instance bookkeeping (blocked, standing).
-- - Reset (event 23): C++ Initialize() clears enraged/protected/charges
--   and RemoveAllAuras clears the soul-charge auras (no RemoveAllAuras
--   bridge — subsumed by the counter reset); _Reset() is instance
--   bookkeeping (blocked, standing).
-- Unmodeled (documented-only, no bridges):
-- - EVENT_DOOMFIRE (Talk SAY_DOOMFIRE (2) + SummonDoomfire): the entire
--   effect is summon choreography — SummonCreature(NPC_DOOMFIRE_SPIRIT
--   18104 at target+15,+15, TEMPSUMMON_TIMED_DESPAWN 27s) and
--   SummonCreature(NPC_DOOMFIRE 18095 at target-15,-15,
--   TEMPSUMMON_TIMED_DESPAWN 27s), JustSummoned pairing via
--   DoomfireSpiritGUID, the doomfire self-casts SPELL_DOOMFIRE_SPAWN
--   32074 + SPELL_DOOMFIRE 31945 and MoveFollows the spirit — no
--   summon-with-position or movement bridge. Timer not armed; the
--   SAY_DOOMFIRE Talk is tied to the unbridgeable choreography.
-- - EVENT_DISTANCE_CHECK 30s -> 5s: instance->GetCreature(
--   DATA_CHANNEL_TARGET) + IsWithinDistInMap(75.0f) -> ACTION_ENRAGE
--   (NPC_CHANNEL_TARGET 22418 is header-named) — instance-data + GUID
--   bridge blocked (standing); enrage then only fires via the 10%
--   DamageTaken leg in this port.
-- - EVENT_SUMMON_WHISP: DoSpawnCreature(NPC_ANCIENT_WISP 17946,
--   rand32()%40, rand32()%40, 0, 0, TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT,
--   15s) every 1500ms, WispCount >= 30 -> me->KillSelf() — no summon
--   bridge. Timer not armed.
-- - ACTION_CHANNEL_WORLD_TREE on JustReachedHome: re-cast of the
--   pre-combat drain (covered at spawn via event 5; evade-home recast
--   documented).
-- - JustSummoned GUID legs (ANCIENT_WISP AttackStart on the boss,
--   DOOMFIRE_SPIRIT GUID capture) — no summon model.
-- - npc_doomfire (NPC_DOOMFIRE 18095): DamageTaken zeroes damage — no
--   bridge; entry unverified (below).
-- - npc_doomfire_targetting (NPC_DOOMFIRE_SPIRIT 18104): the
--   ChangeTargetTimer 5s MoveFollow/MovePoint(0, random-near-40) movement
--   machine + MoveInLineOfSight player GUID capture + DamageTaken
--   zeroing — no movement bridge; entry unverified (below).
-- - npc_ancient_wisp (NPC_ANCIENT_WISP 17946): 1s CheckTimer — casts
--   SPELL_ANCIENT_SPARK 39349 on the instance DATA_ARCHIMONDE GUID,
--   self-casts SPELL_DENOUEMENT_WISP 32124 when the boss is below 2% or
--   dead; Reset grabs the GUID via instance->GetGuidData and sets
--   NON_ATTACKABLE; DamageTaken zeroing — instance GUID bridge blocked;
--   entry unverified (below).
-- - spell_archimonde_drain_world_tree_dummy: SpellScript
--   OnEffectHitTarget (target casts SPELL_DRAIN_WORLD_TREE_TRIGGERED
--   39141 triggered on the caster) — SpellScript handlers not modeled
--   (blocked, standing).
-- - SAY_SOUL_CHARGE (7) is declared in the C++ Texts enum but never
--   Talked — no C++-side Talk to port.
-- - YELL_ARCHIMONDE_INTRO (8) is Talked from the instance script, not
--   this file (instance-model blocked, standing).
-- npc_doomfire / npc_doomfire_targetting / npc_ancient_wisp: NOT
-- registered — zero C++ entry evidence (no hyjal.h constant for
-- 18095/18104/17946; named only in this cpp's enum Summons; instance
-- OnCreatureCreate cases none of them): registering would invent
-- identifiers (gelihast precedent). Their bridgeable arms are documented
-- above for when entries verify.

local ENTRY = 17968

local SAY_AGGRO      = 1
local SAY_AIR_BURST  = 3
local SAY_SLAY       = 4
local SAY_ENRAGE     = 5
local SAY_DEATH      = 6

local SPELL_FEAR                = 31970
local SPELL_AIR_BURST           = 32014
local SPELL_GRIP_OF_THE_LEGION  = 31972
local SPELL_FINGER_OF_DEATH     = 31984
local SPELL_HAND_OF_DEATH       = 35354
local SPELL_PROTECTION_OF_ELUNE = 38528
local SPELL_DRAIN_WORLD_TREE    = 39140

local SPELL_UNLEASH_SOUL_RED    = 32053
local SPELL_UNLEASH_SOUL_YELLOW = 32054
local SPELL_UNLEASH_SOUL_GREEN  = 32057

-- WoW class ids (nefarian convention).
local CLASS_WARRIOR = 1
local CLASS_PALADIN = 2
local CLASS_HUNTER  = 3
local CLASS_ROGUE   = 4
local CLASS_PRIEST  = 5
local CLASS_SHAMAN  = 7
local CLASS_MAGE    = 8
local CLASS_WARLOCK = 9
local CLASS_DRUID   = 11

-- C++ KilledUnit class buckets: priest/paladin/warlock -> red,
-- mage/rogue/warrior -> yellow, druid/shaman/hunter -> green.
local function chargeColor(class)
    if class == CLASS_PRIEST or class == CLASS_PALADIN
            or class == CLASS_WARLOCK then
        return "red"
    elseif class == CLASS_MAGE or class == CLASS_ROGUE
            or class == CLASS_WARRIOR then
        return "yellow"
    elseif class == CLASS_DRUID or class == CLASS_SHAMAN
            or class == CLASS_HUNTER then
        return "green"
    end
    return nil
end

local state = {}
local timers = {}

local function bossState(guid)
    local st = state[guid]
    if not st then
        st = { enraged = false, protected = false,
               charges = { red = 0, yellow = 0, green = 0 } }
        state[guid] = st
    end
    return st
end

local function clearState(guid)
    state[guid] = nil
end

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
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- janalai convention: bounded random alive player.
local function randomPlayerInRange(creature, maxDist, excludeVictim)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local victim = creature:GetVictim()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist
                and not (excludeVictim and victim and p == victim) then
            found[#found + 1] = p
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

-- nefarian convention: unbounded random alive player in the instance.
local function alivePlayersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- nefarian convention: unbounded random alive player in the instance.
-- C++ SelectTarget(Random, 0) for the grip and finger-of-death legs is
-- DefaultTargetSelector(me, dist=0, playerOnly=false, withTank=false, 0):
-- the tank is EXCLUDED via _exception and dist=0 means no range cap
-- (UnitAI.h comment: "if 0: ignored"; UnitAI.cpp:259-285 verified) —
-- so the victim exclusion here is C++-exact; the player-only restriction
-- stays the documented approximation of the uncapped threat-list pick.
local function randomAlivePlayer(creature, excludeVictim)
    local players = alivePlayersInInstance(creature)
    if excludeVictim then
        local victim = creature:GetVictim()
        if victim then
            for i = #players, 1, -1 do
                if players[i] == victim then
                    table.remove(players, i)
                end
            end
        end
    end
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
end

-- C++ EVENT_FEAR: DoCastAOE(31970), non-triggered, init 42s -> 42s
-- (DoCastAOE resolves to self-cast — kazrogal/illidan precedent).
local function onFear(creature, guid)
    creature:CastSpell(creature, SPELL_FEAR)
    schedule(guid, "fear", 42000, function()
        onFear(creature, guid)
    end)
end

-- C++ EVENT_AIR_BURST: Talk(SAY_AIR_BURST) + DoCast(
-- SelectTarget(Random, 1), 32014) — position 1 ("not on tank") has no
-- threat-list bridge, so a random alive player excluding the current
-- victim (azgalor victim-exclusion convention); non-triggered,
-- init 30s -> {25s,40s}.
local function onAirBurst(creature, guid)
    creature:Talk(SAY_AIR_BURST)
    local target = randomPlayerInRange(creature, 100, true)
    if target and not target:IsDead() then
        creature:CastSpell(target, SPELL_AIR_BURST)
    end
    schedule(guid, "airburst", math.random(25000, 40000), function()
        onAirBurst(creature, guid)
    end)
end

-- C++ EVENT_GRIP_OF_THE_LEGION: DoCast(SelectTarget(Random, 0),
-- 31972), non-triggered, init {5s,25s} -> {5s,25s}. The C++ selection
-- excludes the tank (withTank=false in DefaultTargetSelector), so the
-- victim exclusion is C++-exact.
local function onGrip(creature, guid)
    local target = randomAlivePlayer(creature, true)
    if target and not target:IsDead() then
        creature:CastSpell(target, SPELL_GRIP_OF_THE_LEGION)
    end
    schedule(guid, "grip", math.random(5000, 25000), function()
        onGrip(creature, guid)
    end)
end

-- C++ EVENT_FINGER_OF_DEATH: no target within 5 yards (tank excluded
-- from the check, like every SelectTarget in this file) ->
-- DoCast(SelectTarget(Random, 0), 31984) (tank excluded again), re-arm
-- 1s; else re-arm 5s. The second selection is independent (may be nil ->
-- DoCast nil no-op), non-triggered, init 15s.
local function onFingerOfDeath(creature, guid)
    if not randomPlayerInRange(creature, 5, true) then
        local target = randomAlivePlayer(creature, true)
        if target and not target:IsDead() then
            creature:CastSpell(target, SPELL_FINGER_OF_DEATH)
        end
        schedule(guid, "finger", 1000, function()
            onFingerOfDeath(creature, guid)
        end)
    else
        schedule(guid, "finger", 5000, function()
            onFingerOfDeath(creature, guid)
        end)
    end
end

-- C++ EVENT_HAND_OF_DEATH: DoCastAOE(35354), non-triggered,
-- init 10min -> 2s (DoCastAOE resolves to self-cast).
local function onHandOfDeath(creature, guid)
    creature:CastSpell(creature, SPELL_HAND_OF_DEATH)
    schedule(guid, "handofdeath", 2000, function()
        onHandOfDeath(creature, guid)
    end)
end

-- C++ EVENT_UNLEASH_SOUL_CHARGE: urand(0,2) picks a color; if the
-- matching charge aura is stacked, remove one stack and DoCastVictim the
-- matching unleash spell, then re-arm {2s,30s}; no re-arm when absent.
-- Charges are Lua counters (see header note on the unbridgeable
-- victim->CastSpell delivery).
local unleashByColor = {
    red    = SPELL_UNLEASH_SOUL_RED,
    yellow = SPELL_UNLEASH_SOUL_YELLOW,
    green  = SPELL_UNLEASH_SOUL_GREEN,
}

local function onUnleashSoulCharge(creature, guid)
    local st = bossState(guid)
    local pick = math.random(0, 2)
    local color = pick == 0 and "red" or (pick == 1 and "yellow" or "green")
    if st.charges[color] > 0 then
        st.charges[color] = st.charges[color] - 1
        local victim = creature:GetVictim()
        if victim and not victim:IsDead() then
            creature:CastSpell(victim, unleashByColor[color])
        end
        schedule(guid, "unleash", math.random(2000, 30000), function()
            onUnleashSoulCharge(creature, guid)
        end)
    end
end

-- C++ InitializeAI -> DoAction(ACTION_CHANNEL_WORLD_TREE):
-- DoCastAOE(39140, triggered) before combat (storm_cloud OnSpawn(5)
-- convention; DoCastAOE resolves to triggered self-cast).
local function onSpawn(event, creature)
    creature:CastSpell(creature, SPELL_DRAIN_WORLD_TREE, true)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
    bossState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "fear", 42000, function()
        onFear(creature, guid)
    end)
    schedule(guid, "airburst", 30000, function()
        onAirBurst(creature, guid)
    end)
    schedule(guid, "grip", math.random(5000, 25000), function()
        onGrip(creature, guid)
    end)
    schedule(guid, "finger", 15000, function()
        onFingerOfDeath(creature, guid)
    end)
    schedule(guid, "handofdeath", 600000, function()
        onHandOfDeath(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
end

local function onDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    clearState(guid)
    creature:Talk(SAY_DEATH)
end

-- C++ DamageTaken: HealthBelowPctDamaged(10, damage) ->
-- !Enraged -> ACTION_ENRAGE (Talk SAY_ENRAGE; MotionMaster Clear +
-- MoveIdle has no bridge); !HasProtected -> triggered DoCastAOE(38528)
-- (resolves to triggered self-cast) + EVENT_SUMMON_WHISP (no summon
-- bridge — documented, not armed). Event 9 fires pre-damage, so the
-- projected-health check subtracts the incoming damage (aku_mai
-- convention — C++-exact).
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = bossState(guid)
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    if (health - damage) * 100 < 10 * maxHealth then
        if not st.enraged then
            st.enraged = true
            creature:Talk(SAY_ENRAGE)
        end
        if not st.protected then
            st.protected = true
            creature:CastSpell(creature, SPELL_PROTECTION_OF_ELUNE, true)
        end
    end
end

-- C++ KilledUnit: Talk(SAY_SLAY) unconditionally; TYPEID_PLAYER gate
-- (terestian_illhoof convention) -> class-bucketed soul charge gain +
-- ScheduleEvent(EVENT_UNLEASH_SOUL_CHARGE, {2s,30s}).
local function onKilledUnit(event, creature, victim)
    creature:Talk(SAY_SLAY)
    if victim and victim:GetObjectType() == "Player" then
        local color = chargeColor(victim:GetClass())
        if color then
            local guid = creature:GetGUID()
            local st = bossState(guid)
            st.charges[color] = st.charges[color] + 1
            schedule(guid, "unleash", math.random(2000, 30000), function()
                onUnleashSoulCharge(creature, guid)
            end)
        end
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onKilledUnit)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 5, onSpawn)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
