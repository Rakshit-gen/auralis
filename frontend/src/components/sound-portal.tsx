/** An illustrated sound aperture: concentric voices opening into a story. */
export function SoundPortal() {
  return (
    <div className="sound-portal" aria-hidden="true">
      <svg viewBox="0 0 600 460" fill="none" className="sound-portal-art">
        <defs>
          <linearGradient id="portal-thread" x1="120" y1="70" x2="470" y2="400" gradientUnits="userSpaceOnUse">
            <stop stopColor="#a9eee0" /><stop offset=".36" stopColor="#49b9bb" />
            <stop offset=".66" stopColor="#456d8a" /><stop offset="1" stopColor="#efba79" />
          </linearGradient>
          <radialGradient id="portal-halo"><stop stopColor="#54d0cc" stopOpacity=".16" /><stop offset="1" stopColor="#54d0cc" stopOpacity="0" /></radialGradient>
          <linearGradient id="portal-core" x1="220" y1="130" x2="360" y2="340" gradientUnits="userSpaceOnUse"><stop stopColor="#142f34" /><stop offset="1" stopColor="#070e16" /></linearGradient>
        </defs>
        <ellipse cx="300" cy="230" rx="290" ry="220" fill="url(#portal-halo)" />
        <g className="portal-rings">
          {Array.from({ length: 38 }, (_, i) => (
            <ellipse key={i} cx="300" cy="230" rx={104 + i * 3.3} ry={100 + i * 1.7}
              transform={`rotate(${-42 + i * 2.3} 300 230)`}
              stroke="url(#portal-thread)" strokeWidth={i % 6 === 0 ? 1.2 : .65} opacity={.24 + i / 65} />
          ))}
        </g>
        <circle cx="300" cy="230" r="94" fill="url(#portal-core)" stroke="#83d9ce" strokeOpacity=".2" />
        <circle cx="300" cy="230" r="81" stroke="#b4e9d7" strokeOpacity=".08" strokeDasharray="2 7" />
        <g className="portal-voice" stroke="url(#portal-thread)" strokeWidth="3" strokeLinecap="round">
          {Array.from({ length: 19 }, (_, i) => {
            const h = 9 + Math.pow(Math.sin(i * .72 + .3), 2) * 38 * Math.sin((i + 1) / 20 * Math.PI);
            return <line key={i} x1={246 + i * 6} x2={246 + i * 6} y1={230 - h / 2} y2={230 + h / 2} />;
          })}
        </g>
        <path d="M65 230H175M425 230H535" stroke="#8ccfc4" strokeOpacity=".18" strokeDasharray="2 6" />
        <circle cx="88" cy="230" r="3" fill="#deb77e" /><circle cx="512" cy="230" r="3" fill="#87e5d4" />
      </svg>
    </div>
  );
}
