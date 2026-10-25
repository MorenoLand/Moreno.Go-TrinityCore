-- Shade of Akama (Black Temple) — Lua port of
-- src/server/scripts/Outland/BlackTemple/boss_shade_of_akama.cpp
-- (boss_shade_of_akama plus npc_akama_shade's Reset stealth self-cast;
-- npc_ashtongue_channeler, npc_creature_generator_akama,
-- npc_ashtongue_sorcerer, npc_ashtongue_defender, npc_ashtongue_rogue,
-- npc_ashtongue_elementalist, npc_ashtongue_spiritbinder and
-- npc_ashtongue_broken documented below, not registered); black_temple.h:33 (DATA_SHADE_
-- OF_AKAMA = 2, third boss), :42 (DATA_AKAMA_SHADE = 9), :73 (NPC_
-- SHADE_OF_AKAMA = 22841). Creature entry: 22841 Shade of Akama
-- (C++ ScriptName "boss_shade_of_akama" per
-- AddSC_boss_shade_of_akama). One Talk line in the boss AI: the
-- 20%-health DamageTaken yell SAY_LOW_HEALTH=3 (ported below).
-- Eluna creature events: 2 OnLeaveCombat, 4 OnDied, 14 OnHitBySpell
-- (SpellHit), 23 OnReset. Melee is engine-driven in Go (creature
-- combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape (C++-exact for the modeled arms): OnDied: non-
-- triggered self-cast Shade of Akama trigger 40955 (C++ DoCast
-- default is triggered=false — vaelastrasz convention). OnHitBy-
-- Spell(14): hit by 40902 (Akama soul retrieve) -> non-triggered
-- self-cast soul expel channel 40927. OnLeaveCombat(2)/OnReset(23):
-- the C++ Reset arms (immune/non-selectable/stun-emote/walk flags,
-- EVENT_INITIALIZE_SPAWNERS, SummonCreatureGroup) and the evade-
-- side despawn arms have no bridges — nothing modeled.
-- The companion AIs in the same C++ file are not registered — the
-- C++ ScriptNames bind DB-side in creature_template (no TDB in this
-- workspace), so the entries cannot be verified against the bindings
-- from the C++ sources alone (garr firesworn convention; the NPC_
-- ASHTONGUE_CHANNELER=23421 / NPC_ASHTONGUE_BROKEN=23319 /
-- NPC_CREATURE_SPAWNER_AKAMA=23210 header constants do not change
-- this — supremus-volcano convention): npc_akama_shade (gossip
-- intro, soul-channel sequence, chain lightning 39945 / destructive
-- poison 40874 / soul retrieve 40902, broken-free outro sequence —
-- only the Reset stealth self-cast 34189 is registered, below),
-- npc_ashtongue_channeler (soul channel 40401 keepalive on the
-- shade while it carries UNIT_FLAG_NOT_SELECTABLE), npc_creature_
-- generator_akama (wave-B 42035 and sorcerer/defender summon
-- scheduler, left/right sides), npc_ashtongue_sorcerer (shade
-- channeler -> Akama attacker switch), npc_ashtongue_defender
-- (debilitating strike 41178 / heroic strike 41975 / shield bash
-- 41180 / windfury 38229), npc_ashtongue_rogue (debilitating poison
-- 41978 / eviscerate 41177), npc_ashtongue_elementalist (rain of
-- fire 42023 / lightning bolt 42024), npc_ashtongue_spiritbinder
-- (spirit heal 42317 / spirit mend 42025 / chain heal 42027) and
-- npc_ashtongue_broken (outro walk/emote/hail/faction switch).
-- The spell_shade_soul_channel_serverside and spell_shade_soul_
-- channel AuraScripts have no AuraScript bridge (standing AuraScript
-- gap).
-- C++-exact on cast gating: boss_shade_of_akama::UpdateAI
-- (boss_shade_of_akama.cpp:297) has no UNIT_STATE_CASTING gate — arms
-- fire unconditionally in C++ too (the file's only such gate, :756, is
-- npc_ashtongue_sorcerer's, which is not registered; the old "no
-- UNIT_STATE bridge" claim was stale). Deviations from C++: no
-- instance-script model — the DATA_SHADE_OF_AKAMA /
-- DATA_AKAMA_SHADE creature lookups, the SetBossState DONE arm, the
-- akama DoAction(ACTION_SHADE_OF_AKAMA_DEAD) arm and the spawner
-- DoAction(ACTION_START/STOP/DESPAWN_ALL_SPAWNS) arms have no
-- bridge; no MovementInform bridge — the chase-complete phase-two
-- gate and the EVENT_ADD_THREAT arm (threat 41602 every 3.5s) are
-- unmodeled; the SetImmuneToPC / UNIT_FLAG_NOT_SELECTABLE Reset
-- arms are PORTED via the bridged unit-flags surface (see shadeReset) —
-- the MovementInform removal pair stays out (no MovementInform bridge);
-- SetWalk still has no bridge, and the SetFaction arms live in the
-- unregistered channeler/sorcerer AIs (the EMOTE_STATE stun/none arms
-- are PORTED 2026-10-20 via creature:EmoteState); no summon
-- model — the channeler spawns ride SummonCreatureGroup (no bridge),
-- but the JustDied DespawnEntry arm is PORTED in shadeDied via the
-- grid sweep; the EVENT_INITIALIZE_SPAWNERS list fetch rides
-- GetCreaturesInGrid but the arm only stores GUIDs for later DoAction
-- dispatches (no DoAction bridge), so nothing observable ports; the
-- EVENT_EVADE_CHECK arm
-- (EnterEvadeModeIfNeeded / IsInBoundary) has no evade or boundary
-- bridge; the soul-channel SpellHit(40447) event-scheduling and
-- AttackStart-on-Akama arms have no bridge (the emotestate arm is
-- ported — see shadeSpellHit).

local ENTRY_SHADE_OF_AKAMA = 22841

local SAY_LOW_HEALTH = 3

local SPELL_SHADE_OF_AKAMA_TRIGGER = 40955
local SPELL_AKAMA_SOUL_CHANNEL = 40447
local SPELL_AKAMA_SOUL_RETRIEVE = 40902
local SPELL_AKAMA_SOUL_EXPEL_CHANNEL = 40927

local ENTRY_ASHTONGUE_CHANNELER = 23421

-- Declared here (rather than in the akama section below) so shadeDied
-- can sweep it: Lua locals scope from their declaration line.
local ENTRY_AKAMA_SHADE = 23191 -- black_temple.h:88 (NPC_AKAMA_SHADE)

-- C++ npc_akama_shade::DamageTaken: one-shot Talk(SAY_LOW_HEALTH=3) when
-- HealthBelowPct(20) (boss_shade_of_akama.cpp:401-407; _hasYelledOnce
-- cleared in Initialize()/Reset(), so akamaShadeReset clears the Lua
-- flag below). DamageTaken arms fire on lethal hits (C++
-- HealthBelowPctDamaged semantics — yazzai/nefarian convention), so no
-- post-damage guard. This arm belongs to npc_akama_shade (entry 23191),
-- NOT to the boss AI. The SAY_BROKEN_FREE_*/SAY_DEAD lines (0,1,2,4) are
-- also npc_akama_shade's.
local akamaYelledOnce = {}

local function akamaShadeDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    -- C++ DamageTaken HealthBelowPct(20) (boss_shade_of_akama.cpp:401-407,
    -- pre-damage): strict-< against the floored integer threshold (the
    -- old GetHealthPct() < 20 fired up to 1 HP early — maulgar/ironaya
    -- precedent). DamageTaken arms fire on lethal hits (no post-damage
    -- guard — yazzai/nefarian convention).
    local maxHealth = creature:GetMaxHealth()
    if not akamaYelledOnce[guid] and maxHealth > 0
        and creature:GetHealth() < math.floor(maxHealth * 20 / 100) then
        akamaYelledOnce[guid] = true
        creature:Talk(SAY_LOW_HEALTH)
    end
end

-- C++ JustDied: non-triggered self-cast of the shade trigger + the
-- channeler DespawnEntry arm (PORTED in shadeDied via the grid sweep;
-- the akama DoAction legs are PORTED below via the same sweep, while
-- the spawner DoActions and SetBossState DONE arms have no
-- instance/summon bridges).
local function shadeDied(event, creature, killer)
    creature:CastSpell(creature, SPELL_SHADE_OF_AKAMA_TRIGGER)
    -- C++ JustDied then calls akama->AI()->DoAction(ACTION_SHADE_OF_AKAMA_DEAD)
    -- (boss_shade_of_akama.cpp:274-275): _isInCombat=false + CombatStop(true)
    -- + SetFaction(FACTION_ASHTONGUE_DEATHSWORN 1820) in C++ order
    -- (SetWalk/_events.Reset/MovePoint stay out — no motion bridge).
    -- The instance GetCreature lookup is approximated by the same
    -- 100yd grid sweep as the channeler arm below.
    for _, akama in ipairs(creature:GetCreaturesInGrid(
            ENTRY_AKAMA_SHADE, 100)) do
        akama:CombatStop(true)
        akama:SetFaction(1820)
    end
    -- C++ boss_shade_of_akama.cpp:282 JustDied summons.DespawnEntry(NPC_
    -- ASHTONGUE_CHANNELER) via the grid-sweep idiom (illidan
    -- parasitic-shadowfiend precedent; C++ SummonList is exact, the
    -- 100yd sweep is the approximation). C++ order: cast first, then
    -- the akama legs above, then the despawn (spawner DoActions +
    -- SetBossState DONE stay out on the instance bridge).
    for _, channeler in ipairs(creature:GetCreaturesInGrid(
            ENTRY_ASHTONGUE_CHANNELER, 100)) do
        channeler:Despawn()
    end
end

-- C++ SpellHit: hit by the soul-retrieve spell -> non-triggered
-- self-cast of the expel channel. The soul-channel (40447) SpellHit
-- event-scheduling/evade/AttackStart arms are unmodeled (see
-- deviations), but the ungated emotestate arm is C++-exact: it clears
-- the Reset stun visual.
local function shadeSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_AKAMA_SOUL_RETRIEVE then
        creature:CastSpell(creature, SPELL_AKAMA_SOUL_EXPEL_CHANNEL)
    elseif spellId == SPELL_AKAMA_SOUL_CHANNEL then
        -- C++ boss_shade_of_akama.cpp:244: SetUInt32Value(UNIT_NPC_EMOTESTATE, EMOTE_ONESHOT_NONE=0)
        creature:EmoteState(0)
    end
end

local function shadeReset(event, creature)
    -- C++ boss_shade_of_akama.cpp:221: SetUInt32Value(UNIT_NPC_EMOTESTATE, EMOTE_STATE_STUN=64)
    creature:EmoteState(64)
    -- C++ boss_shade_of_akama.cpp:219-220 (Reset): SetImmuneToPC(true)
    -- = SetFlag(59, UNIT_FLAG_IMMUNE_TO_PC 0x100), and SetFlag(59,
    -- UNIT_FLAG_NOT_SELECTABLE 0x2000000) -- both PORTED via the bridged
    -- unit-flags surface (Unit.cpp:8440-8495). The MovementInform
    -- chase-complete removal pair (:259-260) stays out -- no
    -- MovementInform bridge. IsAlive-gated: C++ Reset() never runs on
    -- death, and ev23 fires ahead of ev4 on the death path (the
    -- bladeReset convention in this file).
    if creature:IsAlive() then
        creature:SetFlag(59, 0x100)
        creature:SetFlag(59, 0x2000000)
    end
end

RegisterCreatureEvent(ENTRY_SHADE_OF_AKAMA, 4, shadeDied)
RegisterCreatureEvent(ENTRY_SHADE_OF_AKAMA, 14, shadeSpellHit)
RegisterCreatureEvent(ENTRY_SHADE_OF_AKAMA, 23, shadeReset)

-- Akama shade (npc_akama_shade, C++ ScriptName; black_temple.h:88
-- NPC_AKAMA_SHADE = 23191): the Reset stealth arm and the JustDied
-- Talk(SAY_DEAD) are modeled, and the gossip-intro select legs are
-- PORTED below (akamaShadeGossipSelect) — the soul-channel
-- sequence and combat/outro arms are instance/react/threat-driven
-- and stay unregistered (see above).
-- C++ Reset: DoCastSelf(SPELL_STEALTH 34189), non-triggered (C++
-- DoCast default is triggered=false — vaelastrasz convention).
-- Event 23 also fires on death (reset-then-died order,
-- lua_creature_events.go) but the C++ Reset() never runs on death,
-- so the cast is alive-gated (bladeReset convention in
-- boss_illidan.lua).

local SPELL_STEALTH = 34189

local UNIT_NPC_FLAG_GOSSIP = 0x00000001

-- Timers via CreateLuaEvent (vaelastrasz idiom), GUID-keyed,
-- cancelled on events 4/23 below.
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

-- C++ npc_akama_shade::JustDied (boss_shade_of_akama.cpp:527-534):
-- Talk(SAY_DEAD 4), unconditional — the sjonnir JustDied-Talk
-- precedent. The _summons.DespawnAll() and the cross-creature
-- EnterEvadeMode(EVADE_REASON_OTHER) on the shade ride the
-- instance/summon bridges — documented-only.
local SAY_DEAD = 4

local function akamaShadeDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEAD)
end

local function akamaShadeReset(event, creature)
    -- C++ npc_akama_shade::Reset (boss_shade_of_akama.cpp:364-371):
    -- Initialize() clears _hasYelledOnce, then SetFaction(FACTION_
    -- ASHTONGUE_DEATHSWORN 1820) before the stealth self-cast.
    -- The gossip-flag leg: C++ re-adds UNIT_NPC_FLAG_GOSSIP only when
    -- the instance boss-state is not DONE (no boss-state bridge), so
    -- it is re-added unconditionally here — the approximation the
    -- gossip-select port below depends on (without it the hook can
    -- never fire after the first SHADE_START removal).
    -- SetFaction is bridged (lua_creature_events.go:1125).
    akamaYelledOnce[creature:GetGUID()] = nil
    cancelTimers(creature:GetGUID())
    if creature:IsAlive() then
        creature:SetFaction(1820)
        creature:CastSpell(creature, SPELL_STEALTH)
        creature:SetNPCFlags(bit32.bor(creature:GetNPCFlags(),
            UNIT_NPC_FLAG_GOSSIP))
    end
end

-- C++ npc_akama_shade::OnGossipSelect (boss_shade_of_akama.cpp:536-541):
-- gossipListId 0 -> CloseGossipMenuFor + EVENT_SHADE_START at 500ms.
-- The Go select hook passes (sender, action), not gossipListId; the
-- menu is DB-side, so sender == 0 identifies the DB menu's option
-- (grandmother precedent in bosses_opera.lua). C++ returns false
-- (native select arms proceed).
local function akamaShadeGossipSelect(event, player, creature, sender, action)
    if sender ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    local guid = creature:GetGUID()
    schedule(guid, "shade_start", 500, function()
        -- C++ EVENT_SHADE_START (boss_shade_of_akama.cpp:460-467):
        -- SetBossState IN_PROGRESS (no bridge) + RemoveFlag(UNIT_NPC_FLAGS,
        -- UNIT_NPC_FLAG_GOSSIP) + RemoveAurasDueToSpell(SPELL_STEALTH)
        -- (SetWalk/MovePoint stay out — no motion bridge).
        creature:SetNPCFlags(bit32.band(creature:GetNPCFlags(),
            bit32.bnot(UNIT_NPC_FLAG_GOSSIP)))
        creature:RemoveAura(SPELL_STEALTH)
    end)
    return false
end

RegisterCreatureEvent(ENTRY_AKAMA_SHADE, 4, akamaShadeDied)
RegisterCreatureEvent(ENTRY_AKAMA_SHADE, 9, akamaShadeDamageTaken)
RegisterCreatureEvent(ENTRY_AKAMA_SHADE, 23, akamaShadeReset)
RegisterCreatureGossipEvent(ENTRY_AKAMA_SHADE, 2, akamaShadeGossipSelect)
