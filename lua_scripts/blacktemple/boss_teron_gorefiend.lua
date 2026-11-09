-- Teron Gorefiend (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_teron_gorefiend.cpp
-- (boss_teron_gorefiend only; npc_doom_blossom, npc_shadowy_
-- construct, the shadow_of_death / spiritual_vengeance AuraScripts,
-- the shadow_of_death_remove SpellScript and
-- at_teron_gorefiend_entrance documented below, not registered);
-- black_temple.h:34 (DATA_TERON_GOREFIEND = 3, fourth boss), :74
-- (NPC_TERON_GOREFIEND = 22871). Creature entry: 22871 Teron
-- Gorefiend (C++ ScriptName "boss_teron_gorefiend" per
-- RegisterBlackTempleCreatureAI in AddSC_boss_teron_gorefiend; the
-- creature_template ScriptName binding is DB-side — no TDB in this
-- workspace, so only the C++-side naming is verified). Talk lines
-- used: SAY_AGGRO=1 (pull), SAY_SLAY=2 (kill, player-only),
-- SAY_INCINERATE=3 (incinerate), SAY_BLOSSOM=4 (doom blossom),
-- SAY_CRUSHING=5 (crushing shadows), SAY_DEATH=6 (death);
-- SAY_INTRO=0 belongs to the unmodeled intro arm.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnEnterCombat:
-- Talk(SAY_AGGRO), arm berserk 45078 10min, non-triggered
-- self-cast, one-shot (C++ DoCast default is triggered=false —
-- vaelastrasz convention) / incinerate 40239 12s then {12s,20s},
-- non-triggered on a random alive threat-list unit (C++
-- SelectTarget(Random, 0), playerOnly defaults false, so
-- pets/totems are eligible; GetAITargets threat-list pool)
-- + Talk(SAY_INCINERATE) / summon doom blossom 40188
-- 8s then
-- {30s,40s}, triggered self-cast (C++ DoCastSelf(..., true)) +
-- Talk(SAY_BLOSSOM); the doom blossom arrives via the 40188 spell-effect
-- summon (creatureEffectSummonType, supremus-volcano precedent) and its
-- Reset arms ride the event-19 relay below / shadow of death
-- 40251 8s then {30s,35s}, non-triggered on a random alive
-- threat-list player within 100 yd with the first MaxThreat entry
-- skipped before filters, excluding players carrying 40268
-- (spiritual vengeance; the -aura
-- filter has a target-HasAura bridge here — C++-exact) / crushing
-- shadows 40243 18s then {18s,30s}, non-triggered self-cast + Talk
-- (SAY_CRUSHING); the SPELLVALUE_MAX_TARGETS=5 arm has no bridge —
-- the cast goes out unclamped. Nil-target ticks cast nothing but
-- keep the schedule (jeklik convention). OnTargetDied(3):
-- Talk(SAY_SLAY), TYPEID_PLAYER gate (terestian convention).
-- OnDied(4): Talk(SAY_DEATH); the DoCast(SPELL_SHADOW_OF_DEATH_
-- REMOVE 41999) arm ports as a non-triggered self-cast (plain DoCast
-- default triggered=false — vaelastrasz convention) plus the Lua-side
-- OnHit removal follow-up (40282/40268/40251, hit = self).
-- OnLeaveCombat(2)/OnReset(23): cancel
-- timers. The C++ intro arms (Reset's DATA_TERON_GOREFIEND_INTRO
-- gate, DoAction(ACTION_START_INTRO) + Talk(SAY_INTRO) + 20s
-- EVENT_FINISH_INTRO, the NON_ATTACKABLE/NOT_SELECTABLE/REACT_
-- PASSIVE flag arms) stay unported — flag/react-state bridges exist
-- since 2026-10-20 but the area trigger
-- at_teron_gorefiend_entrance that starts the intro (instance
-- GetCreature(DATA_TERON_GOREFIEND) -> DoAction) has no
-- area-trigger bridge and the intro arms are
-- blocked on the instance-script model (standing gaps), so the
-- fight starts on pull, ragnaros-intro convention. The :147
-- SetData(DATA_TERON_GOREFIEND_INTRO, 0) inside that DoAction stays out
-- with it — the instance stores the value with no side effect
-- (instance_black_temple.cpp:167) and its only reader is Teron's own
-- Reset gate (:117), itself unported. The
-- EnterEvadeMode arms: summons.DespawnAll — creature:SummonCreature IS
-- bridged but the summon-list model is absent (stays out); the
-- DoCast(41999) self-cast arm is ported (teronDied precedent); the
-- _DespawnAtEvade() -> DespawnOrUnsummon(0s,30s) arm rides
-- creature:Despawn in teronLeaveCombat (stale 'no bridge' retracted). The npc_doom_blossom
-- AI (NPC_DOOM_BLOSSOM 23123 per black_temple companion enum: hover
-- takeoff +8z, cast 40186, shadowbolt 40185 on a random target 12s
-- then every 2s) and npc_shadowy_construct AI (NPC_SHADOWY_
-- CONSTRUCT 23111: damage/school spell-immunity arms, atrophy 40327
-- on the victim 12s then {10s,12s}, 1s re-target loop via Teron's
-- threat pick excluding 40268-carrying players with 1,000,000
-- threat) are not registered — the C++ ScriptNames bind DB-side in
-- creature_template (no TDB in this workspace), so the entries
-- cannot be verified against the bindings from the C++ sources
-- alone (garr firesworn convention). The construct's
-- instance-creature lookup, ResetThreatList/AddThreat(1,000,000)
-- and ApplySpellImmune arms stay out — the immunity bridge exists
-- (creature:ApplySpellImmune) but the recv is cross-creature via the
-- instance lookup, with no GUID-to-creature bridge. The
-- spell_teron_gorefiend_shadow_of_death AuraScript: the EFFECT_0
-- absorb arm (PreventDefaultAction) has no bridge and stays out;
-- the on-expire remove arm (summons the spirit + 4 skeletrons +
-- 40282 + 40268) is PORTED below via RegisterAuraEvent ev3, as is
-- the spell_teron_gorefiend_spiritual_vengeance AuraScript
-- (on-remove KillSelf).
-- Cast gating PORTED (mal_ganis idiom): the C++ UpdateAI queue gate
-- (boss_teron_gorefiend.cpp:174) and post-event check (:247) are modeled
-- on every timer tick via the live creature:IsCasting() bridge
-- (engine/world/lua_creature_events.go:1950, CastingUntil window) — a
-- casting tick defers 500ms under its own key, consuming no re-arm.
-- The JustSummoned hook is not an UpdateAI event and stays ungated; the
-- doom blossom / shadowy construct gates belong to unregistered AIs.
-- Deviations from C++ (remaining): no instance-
-- script model — boss admission via the luaBossAI shim
-- (BossAI::JustEngagedWith and the DATA_TERON_GOREFIEND
-- bookkeeping arms skipped).

local ENTRY_TERON_GOREFIEND = 22871

local SAY_AGGRO = 1
local SAY_SLAY = 2
local SAY_INCINERATE = 3
local SAY_BLOSSOM = 4
local SAY_CRUSHING = 5
local SAY_DEATH = 6

local SPELL_INCINERATE = 40239
local SPELL_CRUSHING_SHADOWS = 40243
local SPELL_SHADOW_OF_DEATH = 40251
local SPELL_BERSERK = 45078
local SPELL_SUMMON_DOOM_BLOSSOM = 40188
local SPELL_DOOM_BLOSSOM_SHADOWBOLT = 40185
local SPELL_SUMMON_BLOSSOM_MOVE_TARGET = 40186
local ENTRY_DOOM_BLOSSOM = 23123
local SPELL_SPIRITUAL_VENGEANCE = 40268
local SPELL_POSSESS_SPIRIT_IMMUNE = 40282
local SPELL_SHADOW_OF_DEATH_REMOVE = 41999
local SPELL_SUMMON_SPIRIT = 40266 -- boss_teron_gorefiend.cpp:58
local SPELL_SUMMON_SKELETRON_1 = 40270 -- boss_teron_gorefiend.cpp:61
local SPELL_SUMMON_SKELETRON_2 = 41948 -- boss_teron_gorefiend.cpp:62
local SPELL_SUMMON_SKELETRON_3 = 41949 -- boss_teron_gorefiend.cpp:63
local SPELL_SUMMON_SKELETRON_4 = 41950 -- boss_teron_gorefiend.cpp:64
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67

local timers = {}

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

-- C++ SelectTarget(Random, 0): any alive threat-list unit, no
-- range cap, no player gate (playerOnly defaults false), victim
-- included. Shared by Teron's incinerate and the doom blossom's
-- shadowbolt — each caller passes its own creature, so each reads
-- its own threat list.
local function randomAnyTarget(creature)
    local candidates = {}
    for _, u in ipairs(creature:GetAITargets()) do
        if not u:IsDead() then
            candidates[#candidates + 1] = u
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ SelectTarget(Random, 1, 100.0f, true, true,
-- -SPELL_SPIRITUAL_VENGEANCE): threat-list players alive within
-- 100 yd, minus the victim (UnitAI.h builds the list victim-first,
-- so offset 1 removes the victim, not the threat head) and minus
-- players carrying 40268.
local function randomShadowDeathTarget(creature)
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end

    local targets = creature:GetAITargets()
    local candidates = {}
    local aliveSeen = 0
    for _, p in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = p:GetGUID() == victimGuid
        elseif not p:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and p:GetObjectType() == "Player" and not p:IsDead()
                and creature:GetDistance(p) - creature:GetCombatReach() - 1.5 < 100 -- IsWithinCombatRange gate, reach-exact (Unit.cpp:583; player reach 1.5)
                and not p:HasAura(SPELL_SPIRITUAL_VENGEANCE) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_ENRAGE: non-triggered self-cast 45078; one-shot
-- (C++-exact — no re-arm).
local function onBerserk(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "berserk", 500, function()
            onBerserk(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BERSERK, false)
end

-- C++ EVENT_INCINERATE: non-triggered 40239 on a random target +
-- Talk(SAY_INCINERATE); re-arm {12s,20s} unconditional (C++-exact —
-- the repeat runs even when no target was found).
local function onIncinerate(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "incinerate", 500, function()
            onIncinerate(creature, guid)
        end)
        return
    end
    local target = randomAnyTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_INCINERATE, false)
    end
    creature:Talk(SAY_INCINERATE)
    schedule(guid, "incinerate", 12000 + math.random(0, 8000), function()
        onIncinerate(creature, guid)
    end)
end

-- C++ EVENT_SUMMON_DOOM_BLOSSOM: triggered self-cast 40188 +
-- Talk(SAY_BLOSSOM); re-arm {30s,40s}. The doom blossom itself
-- is summoned by spell 40188's cast effect in C++ (DoCastSelf, no
-- C++ SummonCreature leg), so the cast is the whole modeled arm.
local function onDoomBlossom(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "doomblossom", 500, function()
            onDoomBlossom(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SUMMON_DOOM_BLOSSOM, true)
    creature:Talk(SAY_BLOSSOM)
    schedule(guid, "doomblossom", 30000 + math.random(0, 10000), function()
        onDoomBlossom(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_DEATH: non-triggered 40251 on the filtered
-- random target; re-arm {30s,35s} unconditional (C++-exact).
local function onShadowDeath(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "shadowdeath", 500, function()
            onShadowDeath(creature, guid)
        end)
        return
    end
    local target = randomShadowDeathTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_OF_DEATH, false)
    end
    schedule(guid, "shadowdeath", 30000 + math.random(0, 5000), function()
        onShadowDeath(creature, guid)
    end)
end

-- C++ EVENT_CRUSHING_SHADOWS: non-triggered self-cast 40243 +
-- Talk(SAY_CRUSHING); re-arm {18s,30s}. The SPELLVALUE_MAX_TARGETS
-- = 5 arm has no bridge — the cast goes out unclamped.
local function onCrushingShadows(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "crushing", 500, function()
            onCrushingShadows(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_CRUSHING_SHADOWS, false)
    creature:Talk(SAY_CRUSHING)
    schedule(guid, "crushing", 18000 + math.random(0, 12000), function()
        onCrushingShadows(creature, guid)
    end)
end

local function teronEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    creature:Talk(SAY_AGGRO)
    schedule(guid, "berserk", 600000, function()
        onBerserk(creature, guid)
    end)
    schedule(guid, "incinerate", 12000, function()
        onIncinerate(creature, guid)
    end)
    schedule(guid, "doomblossom", 8000, function()
        onDoomBlossom(creature, guid)
    end)
    schedule(guid, "shadowdeath", 8000, function()
        onShadowDeath(creature, guid)
    end)
    schedule(guid, "crushing", 18000, function()
        onCrushingShadows(creature, guid)
    end)
end

local function teronResetState(guid)
    cancelTimers(guid)
end

local function teronLeaveCombat(event, creature)
    teronResetState(creature:GetGUIDLow())
    -- C++ EnterEvadeMode arm order: DoCast(SPELL_SHADOW_OF_DEATH_
    -- REMOVE 41999) self-cast (same AITARGET_SELF bridge as teronDied)
    -- -> summons.DespawnAll() (no summon-list bridge) -> _DespawnAtEvade()
    -- -> DespawnOrUnsummon(0s, 30s) (ScriptedCreature.cpp:596-614;
    -- najentus/supremus precedent).
    creature:CastSpell(creature, SPELL_SHADOW_OF_DEATH_REMOVE, false)
    -- 41999 OnHit follow-up (boss_teron_gorefiend.cpp:414-421): the
    -- hit unit is self here, so the removals port Lua-side in C++
    -- order: 40282 immune, 40268 vengeance, 40251 shadow of death.
    creature:RemoveAura(SPELL_POSSESS_SPIRIT_IMMUNE)
    creature:RemoveAura(SPELL_SPIRITUAL_VENGEANCE)
    creature:RemoveAura(SPELL_SHADOW_OF_DEATH)
    creature:Despawn()
end

-- C++ KilledUnit: Talk(SAY_SLAY), TYPEID_PLAYER gate.
local function teronTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH) + the plain DoCast(SPELL_SHADOW_
-- OF_DEATH_REMOVE 41999) arm (boss_teron_gorefiend.cpp:163; DoCast
-- self-casts since 41999 carries no enemy-area effect target —
-- UnitAI::FillAISpellInfo leaves AITARGET_SELF). The 41999 OnHit
-- removal arms port Lua-side below (hit = self, C++ order).
local function teronDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    creature:CastSpell(creature, SPELL_SHADOW_OF_DEATH_REMOVE, false)
    -- Same 41999 OnHit follow-up as teronLeaveCombat (hit = self).
    creature:RemoveAura(SPELL_POSSESS_SPIRIT_IMMUNE)
    creature:RemoveAura(SPELL_SPIRITUAL_VENGEANCE)
    creature:RemoveAura(SPELL_SHADOW_OF_DEATH)
    teronResetState(creature:GetGUIDLow())
end

local function teronReset(event, creature)
    teronResetState(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 1, teronEnterCombat)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 2, teronLeaveCombat)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 3, teronTargetDied)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 4, teronDied)
RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 23, teronReset)

-- C++ npc_doom_blossom::Reset (boss_teron_gorefiend.cpp:236-243): 12s
-- init, SelectTarget(Random) -> DoCast(target, SPELL_SHADOWBOLT
-- 40185), Repeat(2s). The blossom arrives via the 40188 spell-effect
-- summon, so its Reset arm rides the boss event-19 relay (supremus
-- volcano precedent): if the spell-effect summon never spawns, this
-- never fires and nothing is fabricated. The +8z lift itself stays out
-- (no MovePoint bridge); the hover leg, the 40186 cast and
-- DoZoneInCombat are PORTED in teronJustSummoned below; the blossom's
-- timers die with Teron's timers on leave/died/reset.
local function blossomShadowboltTick(blossom, bossGuid, blossomGuid)
    local target = randomAnyTarget(blossom)
    if target then
        blossom:CastSpell(target, SPELL_DOOM_BLOSSOM_SHADOWBOLT, false)
    end
    schedule(bossGuid, "blossomShadowbolt:" .. blossomGuid, 2000, function()
        blossomShadowboltTick(blossom, bossGuid, blossomGuid)
    end)
end

local function teronJustSummoned(event, creature, summon)
    if summon:GetEntry() == ENTRY_DOOM_BLOSSOM then
        local bossGuid = creature:GetGUIDLow()
        local blossomGuid = summon:GetGUIDLow()
        -- C++ npc_doom_blossom::Reset (boss_teron_gorefiend.cpp:225-236):
        -- MoveTakeoff +8z -> non-triggered self-cast 40186 -> DoZoneInCombat,
        -- C++ order. The +8z lift itself stays out (no MovePoint bridge);
        -- the hover leg rides SetDisableGravity(true), the takeoff state
        -- twin (the C++ comment frames the takeoff as the hover workaround).
        summon:SetDisableGravity(true)
        summon:CastSpell(summon, SPELL_SUMMON_BLOSSOM_MOVE_TARGET)
        summon:DoZoneInCombat()
        schedule(bossGuid, "blossomShadowbolt:" .. blossomGuid, 12000, function()
            blossomShadowboltTick(summon, bossGuid, blossomGuid)
        end)
    end
end

RegisterCreatureEvent(ENTRY_TERON_GOREFIEND, 19, teronJustSummoned)

-- C++ spell_teron_gorefiend_shadow_of_death::OnRemove
-- (boss_teron_gorefiend.cpp:360-375): AfterEffectRemove, EFFECT_1
-- SPELL_AURA_OVERRIDE_CLASS_SCRIPTS, AURA_EFFECT_HANDLE_REAL,
-- expire-only. The target self-casts 40266 + the four skeletron
-- summons (40270/41948/41949/41950) + 40282 + 40268, all triggered,
-- C++ order. The EFFECT_0 absorb arm (PreventDefaultAction) has no
-- bridge and stays out.
local function shadowOfDeathRemove(event, aura, target)
    if aura:GetRemoveMode() == AURA_REMOVE_BY_EXPIRE then
        target:CastSpell(target, SPELL_SUMMON_SPIRIT, true)
        target:CastSpell(target, SPELL_SUMMON_SKELETRON_1, true)
        target:CastSpell(target, SPELL_SUMMON_SKELETRON_2, true)
        target:CastSpell(target, SPELL_SUMMON_SKELETRON_3, true)
        target:CastSpell(target, SPELL_SUMMON_SKELETRON_4, true)
        target:CastSpell(target, SPELL_POSSESS_SPIRIT_IMMUNE, true)
        target:CastSpell(target, SPELL_SPIRITUAL_VENGEANCE, true)
    end
end

RegisterAuraEvent(SPELL_SHADOW_OF_DEATH, 3, shadowOfDeathRemove)

-- C++ spell_teron_gorefiend_spiritual_vengeance::OnRemove
-- (boss_teron_gorefiend.cpp:386-391): AfterEffectRemove, EFFECT_0
-- SPELL_AURA_MOD_POSSESS, AURA_EFFECT_HANDLE_REAL, any remove mode
-- -> KillSelf (creature:Kill() twin, bridged for player and
-- creature targets, lua_player.go:3426).
local function spiritualVengeanceRemove(event, aura, target)
    target:Kill()
end

RegisterAuraEvent(SPELL_SPIRITUAL_VENGEANCE, 3, spiritualVengeanceRemove)
